using System;
using System.Text;
using System.Net.Http;
using Newtonsoft.Json.Linq;
using Microsoft.AspNetCore.Builder;
using Microsoft.AspNetCore.Http;
using Microsoft.Extensions.Hosting;
using RabbitMQ.Client;
using System.Collections.Generic;
using System.Threading.Tasks;
using Newtonsoft.Json;

var builder = WebApplication.CreateBuilder(args);
// no MVC controllers required for minimal API endpoints; keep JSON support via System.Text.Json or add Newtonsoft where needed
var app = builder.Build();

// Simple in-memory SSE clients list
var sseClients = new List<HttpResponse>();

// Backend does not store projections; it will proxy reads to the projection-cache service.
// No HTTP proxy to projection-cache anymore: backend only forwards SSE and publishes events.

// Start background RabbitMQ consumer to forward projection notifications to SSE clients
var rabbitHost = Environment.GetEnvironmentVariable("RABBITMQ_HOST") ?? "rabbitmq";
var factory = new ConnectionFactory() { HostName = rabbitHost };
var rabbitConn = factory.CreateConnection();
var rabbitChannel = rabbitConn.CreateModel();
// Bind to the projections-ex fanout exchange so backend receives projection notifications for SSE
var exch = "projections-ex";
rabbitChannel.ExchangeDeclare(exch, ExchangeType.Fanout, durable: true, autoDelete: false, arguments: null);
var qBack = rabbitChannel.QueueDeclare("projections-backend", durable: true, exclusive: false, autoDelete: false, arguments: null);
    rabbitChannel.QueueBind(qBack.QueueName, exch, string.Empty, null);
var consumer = new RabbitMQ.Client.Events.EventingBasicConsumer(rabbitChannel);
consumer.Received += (model, ea) => {
    var body = ea.Body.ToArray();
    var json = Encoding.UTF8.GetString(body);
    Console.WriteLine($"backend: received projection message: {json}");
    // broadcast to SSE clients using async writes to avoid blocking the consumer thread
    lock(sseClients)
    {
        foreach(var resp in sseClients.ToArray())
        {
            var r = resp;
            Task.Run(async () => {
                try {
                    var data = $"data: {json}\n\n";
                    var bytes = Encoding.UTF8.GetBytes(data);
                    await r.Body.WriteAsync(bytes, 0, bytes.Length);
                    await r.Body.FlushAsync();
                } catch (Exception ex) {
                    Console.WriteLine($"backend: SSE write failed, removing client: {ex.Message}");
                    lock(sseClients) { sseClients.Remove(r); }
                }
            });
        }
    }
};
rabbitChannel.BasicConsume(queue: qBack.QueueName, autoAck: true, consumer: consumer);


// POST /api/events -> push to RabbitMQ and store event in events table can be done by consumer
app.MapPost("/api/events", async (HttpContext ctx) =>
{
    var body = await new System.IO.StreamReader(ctx.Request.Body).ReadToEndAsync();
    // attach an eventId if caller did not provide one
    string enriched = body;
    try {
        var jo = Newtonsoft.Json.Linq.JObject.Parse(body);
        if (jo["eventId"] == null || string.IsNullOrEmpty((string)jo["eventId"])) {
            jo["eventId"] = Guid.NewGuid().ToString();
        }
        enriched = jo.ToString(Newtonsoft.Json.Formatting.None);
    } catch {
        // ignore parse errors and send original body
    }
    Console.WriteLine($"POST /api/events publishing event: {enriched}");
    var connFactory = new ConnectionFactory() { HostName = Environment.GetEnvironmentVariable("RABBITMQ_HOST") ?? "rabbitmq" };
    using(var conn = connFactory.CreateConnection())
    using(var channel = conn.CreateModel())
    {
        channel.QueueDeclare("events", durable:true, exclusive:false, autoDelete:false, arguments:null);
        var bytes = Encoding.UTF8.GetBytes(enriched);
        var props = channel.CreateBasicProperties();
        props.Persistent = true;
        channel.BasicPublish(exchange:"", routingKey:"events", basicProperties:props, body:bytes);
    }
    return Results.Accepted();
});

// NOTE: projection reads are no longer exposed via HTTP on the backend.
// The frontend is expected to use the SSE `/events` endpoint to receive projection updates
// and maintain local state. This keeps the backend strictly an event ingress + SSE forwarder.

// SSE endpoint: clients connect here to receive projection notifications
app.MapGet("/events", async (HttpContext ctx) =>
{
    var resp = ctx.Response;
    // use strongly-typed properties where available
    resp.ContentType = "text/event-stream";
    resp.Headers["Cache-Control"] = "no-cache";
    resp.Headers["Connection"] = "keep-alive";

    Console.WriteLine($"backend: SSE client connecting {ctx.Connection.RemoteIpAddress}:{ctx.Connection.RemotePort}");

    // start the response so headers are sent immediately
    try {
        await resp.StartAsync();
    } catch (Exception ex) {
        Console.WriteLine($"backend: failed to start SSE response: {ex.Message}");
        return;
    }

    // Attempt to fetch a snapshot from the projection-cache and stream it to the client
    try {
        var projectionCacheUrl = Environment.GetEnvironmentVariable("PROJECTION_CACHE_URL") ?? "http://projection-cache:8081";
        using var http = new HttpClient();
        http.Timeout = TimeSpan.FromSeconds(3);
        Console.WriteLine($"backend: fetching snapshot from {projectionCacheUrl}/projection");
        var r = await http.GetAsync(projectionCacheUrl + "/projection");
        if (r.IsSuccessStatusCode) {
            var body = await r.Content.ReadAsStringAsync();
            try {
                var arr = JArray.Parse(body);
                foreach (var it in arr) {
                    try {
                        var id = it["id"]?.ToString();
                        var proj = it["projection"] as JToken ?? it["projection"];
                        var evtId = it["metadata"]?["eventId"]?.ToString();
                        var msgObj = new JObject();
                        if (!string.IsNullOrEmpty(id)) msgObj["aggregateId"] = id;
                        msgObj["type"] = "Snapshot";
                        msgObj["projection"] = proj ?? new JObject();
                        if (!string.IsNullOrEmpty(evtId)) msgObj["eventId"] = evtId; // top-level for reconciliation
                        var data = $"data: {msgObj.ToString(Newtonsoft.Json.Formatting.None)}\n\n";
                        var bytes = Encoding.UTF8.GetBytes(data);
                        await resp.Body.WriteAsync(bytes, 0, bytes.Length);
                        await resp.Body.FlushAsync();
                    } catch (Exception ex) {
                        Console.WriteLine($"backend: snapshot write item failed: {ex.Message}");
                    }
                }
                // after streaming all snapshot items, send a SnapshotComplete event so clients can stop loading state
                try {
                    var complete = new JObject();
                    complete["type"] = "SnapshotComplete";
                    var completeData = $"data: {complete.ToString(Newtonsoft.Json.Formatting.None)}\n\n";
                    var completeBytes = Encoding.UTF8.GetBytes(completeData);
                    await resp.Body.WriteAsync(completeBytes, 0, completeBytes.Length);
                    await resp.Body.FlushAsync();
                } catch (Exception ex) {
                    Console.WriteLine($"backend: failed to send SnapshotComplete: {ex.Message}");
                }
            } catch (Exception ex) {
                Console.WriteLine($"backend: snapshot parse error: {ex.Message}");
            }
        } else {
            Console.WriteLine($"backend: projection-cache returned {r.StatusCode}");
        }
    } catch (Exception ex) {
        Console.WriteLine($"backend: snapshot fetch error: {ex.Message}");
    }

    // Now add to live SSE clients so the client receives future projection events
    lock(sseClients) { sseClients.Add(resp); }

    // Keep the connection open until client disconnects
    try {
        await Task.Delay(-1, ctx.RequestAborted);
    } catch (OperationCanceledException) {
        // expected when client disconnects
    } catch (Exception ex) {
        Console.WriteLine($"backend: SSE wait error: {ex.Message}");
    } finally {
        lock(sseClients) { sseClients.Remove(resp); }
        Console.WriteLine($"backend: SSE client disconnected {ctx.Connection.RemoteIpAddress}:{ctx.Connection.RemotePort}");
    }
});

app.Run();
