// store.tsx
import React, { createContext, useContext, useReducer, useEffect, ReactNode } from 'react'
import { v4 as uuidv4 } from 'uuid'
import { sendAndHandle } from './worker'
import { Todo, State, DomainAction } from './types'
import { reducer, initialState } from './reducer'

const StoreContext = createContext<{
  state: State
  dispatch: (action: DomainAction) => void
}>({ state: initialState, dispatch: () => null })

export function StoreProvider({ children }: { children: ReactNode }) {
  const [state, baseDispatch] = useReducer(reducer, initialState)

  // --- Domain Dispatcher ---
  const dispatch = async (action: DomainAction) => {
    switch (action.type) {
      case 'CREATE_TODO': {
        const eventId = uuidv4()
        const tempId = uuidv4() // temporary local id until SSE comes back
        const pending: Todo = {
          id: tempId,
          title: action.title,
          completed: false,
          _pendingAdd: true,
          _eventId: eventId
        }
        baseDispatch({ type: 'ADD_PENDING', todo: pending })

        await sendAndHandle(
          { type: 'TodoCreated', eventId, payload: { title: action.title } },
          baseDispatch
        )
        break
      }

      case 'TOGGLE_TODO': {
        const eventId = uuidv4()
        // Mark existing todo as pending toggle
        baseDispatch({
          type: 'UPDATE_PENDING',
          id: action.id,
          changes: { completed: action.completed, _pendingToggle: true, _eventId: eventId }
        })

        await sendAndHandle(
          { type: 'TodoUpdated', eventId, payload: { id: action.id, completed: action.completed } },
          baseDispatch
        )
        break
      }

      case 'DELETE_TODO': {
        const eventId = uuidv4()
        // Mark existing todo as pending delete
        baseDispatch({ type: 'MARK_DELETE_PENDING', id: action.id })

        await sendAndHandle(
          { type: 'TodoRemoved', eventId, payload: { id: action.id } },
          baseDispatch
        )
        break
      }

      case 'RETRY_CONNECT': {
        baseDispatch({ type: 'RESET_CONNECTING' })
        // useEffect reconnect will kick in
        break
      }
    }
  }

  // --- SSE lifecycle ---
  useEffect(() => {
    let es: EventSource | null = null

    function connect() {
      baseDispatch({ type: 'RESET_CONNECTING' });
      es = new EventSource('/events');

      es.onmessage = (ev) => {
        try {
          const msg = JSON.parse(ev.data);

          if (Array.isArray(msg)) {
            msg.forEach((todo) => {
              baseDispatch({ type: 'ADD_OR_UPDATE', todo: { ...todo, _eventId: todo.id } });
            });
            baseDispatch({ type: 'MARK_OPEN' });
          } else if (msg && typeof msg === 'object' && 'id' in msg) {
            if (msg.type === 'remove') {
              baseDispatch({ type: 'REMOVE', id: msg.id });
            } else {
              // Create the todo object and use the 'id' from the server as the _eventId for correlation
              const todo = {
                id: msg.id,
                title: msg.title,
                completed: msg.completed,
                _eventId: msg.id,
              };
              baseDispatch({ type: 'ADD_OR_UPDATE', todo });
            }
          }
        } catch (e) {
          console.error('sse parse error:', e);
          baseDispatch({ type: 'MARK_ERROR' });
        }
      };

      es.onerror = (e) => {
        console.error('SSE error:', e);
        baseDispatch({ type: 'MARK_ERROR' });
        es?.close();
      };
    }

    connect()
    return () => es?.close()
  }, [])

  return (
    <StoreContext.Provider value={{ state, dispatch }}>
      {children}
    </StoreContext.Provider>
  )
}

export function useStore() {
  return useContext(StoreContext)
}