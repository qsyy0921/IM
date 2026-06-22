import type { Plugin } from "vite";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

type HeaderValue = number | string | readonly string[];

interface HeaderWritable {
  setHeader(name: string, value: HeaderValue): unknown;
}

export default defineConfig({
  plugins: [utf8TextResponseHeaders(), react()],
  server: {
    host: "0.0.0.0",
    port: 5173
  },
  preview: {
    host: "0.0.0.0",
    port: 4173
  }
});

function utf8TextResponseHeaders(): Plugin {
  return {
    name: "nexusim-utf8-text-response-headers",
    configureServer(server) {
      server.middlewares.use((_request, response, next) => {
        installUTF8HeaderHook(response);
        next();
      });
    },
    configurePreviewServer(server) {
      server.middlewares.use((_request, response, next) => {
        installUTF8HeaderHook(response);
        next();
      });
    }
  };
}

function installUTF8HeaderHook(response: HeaderWritable): void {
  const setHeader = response.setHeader.bind(response);
  response.setHeader = ((name: string, value: number | string | readonly string[]) => {
    if (name.toLowerCase() === "content-type" && typeof value === "string") {
      return setHeader(name, withUTF8Charset(value));
    }
    return setHeader(name, value);
  }) as HeaderWritable["setHeader"];
}

function withUTF8Charset(contentType: string): string {
  if (/\bcharset=/i.test(contentType) || !isTextContentType(contentType)) {
    return contentType;
  }
  return `${contentType}; charset=utf-8`;
}

function isTextContentType(contentType: string): boolean {
  return /^(text\/html|text\/css|text\/javascript|application\/javascript|application\/json|image\/svg\+xml)\b/i.test(
    contentType
  );
}
