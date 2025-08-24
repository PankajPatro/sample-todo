import React from 'react'
import { Todo } from '../store/types'

type Props = {
    todo: Todo
    onToggle: (id: string, completed: boolean) => void
    onDelete: (id: string) => void
}

export default function TodoItem({ todo, onToggle, onDelete }: Props) {
  const isPending =
    todo._pendingAdd || todo._pendingToggle || todo._pendingDelete

  return (
    <div
      className={`flex items-center justify-between p-2 border-b ${
        isPending ? 'opacity-50 pointer-events-none' : ''
      }`}
    >
      <label className="flex items-center gap-3">
        <input
          type="checkbox"
          checked={!!todo.completed}
          disabled={isPending}
          onChange={e => onToggle(todo.id, e.target.checked)}
          className="w-4 h-4"
        />
        <span
          className={
            'select-none ' +
            (todo.completed ? 'line-through text-gray-400' : '')
          }
        >
          {todo.title}
        </span>
      </label>
      <button
        aria-label="Delete"
        disabled={isPending}
        onClick={() => onDelete(todo.id)}
        className="text-red-500 hover:text-red-700 disabled:opacity-50 disabled:cursor-not-allowed"
      >
        ✕
      </button>
    </div>
  )
}

