import { State, ReducerAction } from './types'

export const initialState: State = {
  todos: [],
  connection: 'connecting'
}

export function reducer(state: State, action: ReducerAction): State {
  switch (action.type) {
    case 'ADD_PENDING':
      return {
        ...state,
        todos: [action.todo, ...state.todos]
      }

    case 'UPDATE_PENDING':
      return {
        ...state,
        todos: state.todos.map(t =>
          t.id === action.id ? { ...t, ...action.changes } : t
        )
      }

    case 'MARK_DELETE_PENDING':
      return {
        ...state,
        todos: state.todos.map(t =>
          t.id === action.id ? { ...t, _pendingDelete: true } : t
        )
      }

    case 'ADD_OR_UPDATE': {
      const existing = state.todos.find(t => t.id === action.todo.id)
      const updated = existing ? { ...existing, ...action.todo } : action.todo

      // cleanup pending flags
      delete updated._pendingAdd
      delete updated._pendingToggle
      delete updated._pendingDelete

      let filtered = state.todos.filter(t => t.id !== updated.id)
      if (updated._eventId) {
        filtered = filtered.filter(t => t._eventId !== updated._eventId)
      }

      return { ...state, todos: [updated, ...filtered] }
    }

    case 'REMOVE': {
      return {
        ...state,
        todos: state.todos.filter(t => t.id !== action.id)
      }
    }

    case 'MARK_OPEN':
      return { ...state, connection: 'open' }

    case 'MARK_ERROR':
      return { ...state, connection: 'error' }

    case 'RESET_CONNECTING':
      return { ...state, connection: 'connecting' }

    default:
      return state
  }
}