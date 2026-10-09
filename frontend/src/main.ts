import './style.css'
import { mount } from 'svelte'
import App from './App.svelte'
import { theme } from './lib/theme.svelte'

theme.init()

export default mount(App, { target: document.getElementById('app')! })
