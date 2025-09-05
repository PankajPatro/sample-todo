// reducer.tsx
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
        todos: [action.todo, ...state.todos],
      };

    case 'UPDATE_PENDING':
      return {
        ...state,
        todos: state.todos.map((t) =>
          t.id === action.id ? { ...t, ...action.changes } : t
        ),
      };

    case 'MARK_DELETE_PENDING':
      return {
        ...state,
        todos: state.todos.map((t) =>
          t.id === action.id ? { ...t, _pendingDelete: true } : t
        ),
      };

    case 'ADD_OR_UPDATE': {
      const serverTodo = action.todo;

      // Filter out the temporary todo by its _eventId
      const todosWithoutPending = state.todos.filter(
        (t) => t._eventId !== serverTodo._eventId
      );

      // Filter out the old version of the permanent todo by its ID
      const todosFilteredById = todosWithoutPending.filter(
        (t) => t.id !== serverTodo.id
      );

      return {
        ...state,
        todos: [serverTodo, ...todosFilteredById],
      };
    }

    case 'REMOVE': {
      return {
        ...state,
        todos: state.todos.filter((t) => t.id !== action.id),
      };
    }

    case 'MARK_OPEN':
      return { ...state, connection: 'open' };

    case 'MARK_ERROR':
      return { ...state, connection: 'error' };

    case 'RESET_CONNECTING':
      return { ...state, connection: 'connecting' };

    default:
      return state;
  }
}