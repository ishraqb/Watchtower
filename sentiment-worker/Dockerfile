# Compile the TypeScript, then run it with prod-only deps.
# Build context is the sentiment-worker/ directory.
FROM node:22-alpine AS build
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM node:22-alpine
WORKDIR /app
COPY package*.json ./
RUN npm ci --omit=dev
COPY --from=build /app/dist ./dist

ENV NODE_ENV=production
# Render injects PORT; this is just the local default for the health server.
EXPOSE 3001
CMD ["node", "dist/index.js"]
