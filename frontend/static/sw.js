// Minimal service worker for PWA install support.
// This enables the browser's "Add to Home Screen" / "Install App" prompt.
// No offline caching — BirdNET-Go runs on a local network.
self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', event => event.waitUntil(self.clients.claim()));
