import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { SettingsShell } from './SettingsShell'

createRoot(document.getElementById('root')!).render(<StrictMode><SettingsShell /></StrictMode>)
