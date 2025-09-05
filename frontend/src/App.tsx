import React, { useState, FormEvent } from 'react'
import { StoreProvider, useStore } from './store/store'
import TodoList from './components/TodoList'

function InnerApp() {
  const [title, setTitle] = useState<string>('')
  const { state, dispatch } = useStore()

  function add(e: FormEvent) {
    e.preventDefault()
    if (!title) return
    dispatch({ type: 'CREATE_TODO', title })
    setTitle('')
  }

  return (
    <div className="p-6 max-w-xl mx-auto">
      <h2 className="text-2xl font-semibold mb-4">Todo (Event Sourced)</h2>

      <form onSubmit={add} className="flex gap-2">
        <input
          className="flex-1 border rounded px-3 py-2"
          value={title}
          onChange={e => setTitle(e.target.value)}
          placeholder="New todo"
        />
        <button className="bg-blue-600 text-white px-4 py-2 rounded">Add</button>
      </form>

      {state.connection === 'connecting' && (
        <div className="mt-3 italic text-gray-500">
          Connecting to server for live updates...
        </div>
      )}
      {state.connection === 'error' && (
        <div className="mt-3 text-red-500">
          Connection failed. <button onClick={() => dispatch({ type: 'RETRY_CONNECT' })}>Retry</button>
        </div>
      )}

      <div className="mt-4">
        <TodoList
          todos={state.todos}
          onToggle={(id, completed) => dispatch({ type: 'TOGGLE_TODO', id, completed })}
          onDelete={id => dispatch({ type: 'DELETE_TODO', id })}
        />
      </div>
    </div>
  )
}

export default function App() {
  return (
    <StoreProvider>
      <InnerApp />
    </StoreProvider>
  )
}
