import { mount } from 'svelte';
import '@fontsource/libre-caslon-text/400.css';
import '@fontsource/libre-caslon-text/400-italic.css';
import '@fontsource/libre-caslon-text/700.css';
import '@fontsource-variable/libre-franklin';
import './styles/tokens.css';
import './styles/base.css';
import App from './App.svelte';

mount(App, { target: document.getElementById('app') });
