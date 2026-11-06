import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { App } from './App'
import './index.css'

const container = document.getElementById('root')
if (!container) throw new Error('#root not found')

// No StrictMode: its double-invoked effects would start each video twice and double-report the
// playback impression, which is visible in the metrics this project exists to produce.
createRoot(container).render(
  <BrowserRouter>
    <App />
  </BrowserRouter>,
)
