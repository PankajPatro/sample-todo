import React from 'react'
import TodoItem from './TodoItem'
import { Todo } from '../store/types'

type Props = {
  todos: Todo[]
  onToggle: (id: string, completed: boolean) => void
  onDelete: (id: string) => void
}

export default function TodoList({ todos, onToggle, onDelete }: Props) {
  if (todos.length === 0) {
    return <div className="p-4 text-gray-500">No todos yet.</div>
  }

  return (
    <div className="mt-4 border rounded-md overflow-hidden">
      {todos.map(todo => (
        <TodoItem
          key={todo.id}
          todo={todo}
          onToggle={onToggle}
          onDelete={onDelete}
        />
      ))}
    </div>
  )
}
