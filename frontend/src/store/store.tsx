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
      baseDispatch({ type: 'RESET_CONNECTING' })
      es = new EventSource('/events')

      es.onmessage = ev => {
        try {
          const msg = JSON.parse(ev.data)
          const type = msg.type || 'Projection'

          if (type === 'Snapshot' || (msg.projection && msg.projection.id)) {
            const proj = msg.projection
            const eventId = msg.eventId || (msg.metadata && msg.metadata.eventId)

            if (proj && proj.id) {
              const todo: Todo = {
                id: proj.id,
                completed: proj.completed,
                _eventId: eventId
              }
              if (proj.title) todo.title = proj.title
              baseDispatch({ type: 'ADD_OR_UPDATE', todo })
            } else if (proj === null) {
              const id = msg.aggregateId || (msg.projection && msg.projection.id)
              if (id) baseDispatch({ type: 'REMOVE', id })
            }
          } else if (type === 'TodoRemoved') {
            const id = msg.payload?.id || msg.aggregateId
            if (id) baseDispatch({ type: 'REMOVE', id })
          } else if (type === 'SnapshotComplete') {
            baseDispatch({ type: 'MARK_OPEN' })
          }
        } catch (e) {
          console.error('sse parse', e)
        }
      }

      es.onerror = () => {
        baseDispatch({ type: 'MARK_ERROR' })
        es?.close()
      }
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