using System;
using System.Text;
using Microsoft.AspNetCore.Builder;
using Microsoft.AspNetCore.Http;
using RabbitMQ.Client;
using System.Collections.Generic;
using System.Threading.Tasks;
using Grpc.Net.Client;
using Projection;
using Grpc.Core;
using System.Linq;
using System.Text.Json;
using System.Threading.Channels;
using Google.Protobuf.WellKnownTypes;
using System.Collections.Concurrent;

internal class Program
{
    private static void Main(string[] args)
    {
        var builder = WebApplication.CreateBuilder(args);
        // no MVC controllers required for minimal API endpoints; keep JSON support via System.Text.Json or add Newtonsoft where needed
        var app = builder.Build();

        // Simple in-memory SSE clients list
        var sseClients = new List<HttpResponse>();


        // POST /api/events -> push to RabbitMQ and store event in events table can be done by consumer
        app.MapPost("/api/events", async (HttpContext ctx) =>
        {
            var body = await new System.IO.StreamReader(ctx.Request.Body).ReadToEndAsync();
            // attach an eventId if caller did not provide one
            string enriched = body;
            try
            {
                var jo = Newtonsoft.Json.Linq.JObject.Parse(body);
                if (jo["eventId"] == null || string.IsNullOrEmpty((string)jo["eventId"]))
                {
                    jo["eventId"] = Guid.NewGuid().ToString();
                }
                enriched = jo.ToString(Newtonsoft.Json.Formatting.None);
            }
            catch
            {
                // ignore parse errors and send original body
            }
            Console.WriteLine($"POST /api/events publishing event: {enriched}");
            var connFactory = new ConnectionFactory() { HostName = Environment.GetEnvironmentVariable("RABBITMQ_HOST") ?? "rabbitmq" };
            using (var conn = connFactory.CreateConnection())
            using (var channel = conn.CreateModel())
            {
                channel.QueueDeclare("events", durable: true, exclusive: false, autoDelete: false, arguments: null);
                var bytes = Encoding.UTF8.GetBytes(enriched);
                var props = channel.CreateBasicProperties();
                props.Persistent = true;
                channel.BasicPublish(exchange: "", routingKey: "events", basicProperties: props, body: bytes);
            }
            return Results.Accepted();
        });


        // Track active subscribers
        var subscribers = new ConcurrentBag<Channel<string>>();

        // Maintain current projection state
        var currentTodos = new ConcurrentDictionary<string, (string title, bool completed)>();

        // Start background gRPC listener
        _ = Task.Run(async () =>
        {
            var projectionCacheUrl = Environment.GetEnvironmentVariable("PROJECTION_CACHE_URL") ?? "http://localhost:50051";
            using var channel = GrpcChannel.ForAddress(projectionCacheUrl);
            var client = new ProjectionService.ProjectionServiceClient(channel);

            try
            {
                using var call = client.Subscribe(new Empty());

                await foreach (var projectionEvent in call.ResponseStream.ReadAllAsync())
                {
                    string? sseMessage = null;

                    switch (projectionEvent.EventCase)
                    {
                        case ProjectionEvent.EventOneofCase.Snapshot:
                            // Replace all state
                            currentTodos.Clear();
                            foreach (var t in projectionEvent.Snapshot.Todos)
                            {
                                currentTodos[t.Id] = (t.Title, t.Completed);
                            }
                            sseMessage = $"data: {JsonSerializer.Serialize(currentTodos.Select(kv => new { id = kv.Key, title = kv.Value.title, completed = kv.Value.completed }))}\n\n";
                            break;

                        case ProjectionEvent.EventOneofCase.TodoUpdated:
                            var todo = projectionEvent.TodoUpdated;
                            currentTodos[todo.Id] = (todo.Title, todo.Completed);
                            sseMessage = $"data: {JsonSerializer.Serialize(new { id = todo.Id, title = todo.Title, completed = todo.Completed })}\n\n";
                            break;

                        case ProjectionEvent.EventOneofCase.TodoRemovedId:
                            currentTodos.TryRemove(projectionEvent.TodoRemovedId, out _);
                            var removed = new { id = projectionEvent.TodoRemovedId, type = "remove" };
                            sseMessage = $"data: {JsonSerializer.Serialize(removed)}\n\n";
                            break;
                    }

                    if (!string.IsNullOrEmpty(sseMessage))
                    {
                        Console.WriteLine($"Broadcasting SSE: {sseMessage.Trim()}");

                        foreach (var sub in subscribers)
                        {
                            await sub.Writer.WriteAsync(sseMessage);
                        }
                    }
                }
            }
            catch (Exception ex)
            {
                Console.WriteLine($"gRPC listener error: {ex.Message}");
            }
        });

        // SSE endpoint
        app.MapGet("/events", async (HttpContext context) =>
        {
            context.Response.ContentType = "text/event-stream";
            context.Response.Headers.CacheControl = "no-cache";

            var clientChannel = Channel.CreateUnbounded<string>();
            subscribers.Add(clientChannel);

            var cancellation = context.RequestAborted;

            // Send current state immediately as snapshot
            var snapshot = $"data: {JsonSerializer.Serialize(currentTodos.Select(kv => new { id = kv.Key, title = kv.Value.title, completed = kv.Value.completed }))}\n\n";
            await clientChannel.Writer.WriteAsync(snapshot);

            try
            {
                await foreach (var message in clientChannel.Reader.ReadAllAsync(cancellation))
                {
                    await context.Response.WriteAsync(message, cancellation);
                    await context.Response.Body.FlushAsync(cancellation);
                }
            }
            catch (OperationCanceledException)
            {
                Console.WriteLine("Client disconnected.");
            }
        });

        app.Run();
    }
}