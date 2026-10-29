import { mount } from 'svelte';
import '@fontsource-variable/archivo/wdth.css';
import './styles/tokens.css';
import './styles/base.css';
import App from './App.svelte';

mount(App, { target: document.getElementById('app') });
