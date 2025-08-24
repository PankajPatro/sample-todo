export type Todo = {
  id: string
  title?: string
  completed: boolean

  // pending flags
  _pendingAdd?: boolean
  _pendingToggle?: boolean
  _pendingDelete?: boolean

  _eventId?: string
}

export type ConnectionState = 'connecting' | 'open' | 'error'

export type State = {
  todos: Todo[]
  connection: ConnectionState
}

// Reducer (internal) Actions
export type ReducerAction =
  | { type: 'ADD_PENDING'; todo: Todo }
  | { type: 'UPDATE_PENDING'; id: string; changes: Partial<Todo> }
  | { type: 'MARK_DELETE_PENDING'; id: string }
  | { type: 'ADD_OR_UPDATE'; todo: Todo }
  | { type: 'REMOVE'; id: string }
  | { type: 'MARK_OPEN' }
  | { type: 'MARK_ERROR' }
  | { type: 'RESET_CONNECTING' }

// Domain (UI-level) Actions
export type DomainAction =
  | { type: 'CREATE_TODO'; title: string }
  | { type: 'TOGGLE_TODO'; id: string; completed: boolean }
  | { type: 'DELETE_TODO'; id: string }
  | { type: 'RETRY_CONNECT' }
