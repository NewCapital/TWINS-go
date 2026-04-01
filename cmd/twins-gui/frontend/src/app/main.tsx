import React from 'react';
import { createRoot } from 'react-dom/client';
import '../style.css';
import '../styles/qt-theme.css'; // Import Qt theme last to override everything
// Initialize i18n before App renders
import '../i18n/config';
import App from './App';

const container = document.getElementById('root');

const root = createRoot(container!);

root.render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
