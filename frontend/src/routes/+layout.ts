// All the data is fetched client-side from the Go backend (over REST + a
// websocket), so there's nothing to server-render. Prerender the route shells
// as static files and let the browser take over - this is what lets the whole
// thing run on a free static host with no Node server.
export const prerender = true;
export const ssr = false;
