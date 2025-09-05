import React from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'
import './style.css'

const container = document.getElementById('root') as HTMLElement | null
if(container){
  const root = createRoot(container)
  root.render(<App />)
}
