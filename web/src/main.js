import { mount } from 'svelte';
import '@fontsource-variable/archivo/wdth.css';
import './styles/tokens.css';
import './styles/base.css';
import App from './App.svelte';

// WebKit pinch-zooms past touch-action and user-scalable=no; its own gesture
// events are the way to stop it (styles/base.css).
for (const type of ['gesturestart', 'gesturechange', 'gestureend']) {
  document.addEventListener(type, (e) => e.preventDefault(), { passive: false });
}

mount(App, { target: document.getElementById('app') });
