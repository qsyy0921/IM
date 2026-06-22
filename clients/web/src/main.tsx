import React from "react";
import ReactDOM from "react-dom/client";
import { App } from "./App";
import { registerBrowserPWA } from "./pwa";
import "./styles.css";

registerBrowserPWA();

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
