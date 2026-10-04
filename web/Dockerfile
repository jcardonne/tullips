FROM node:22-alpine AS build
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build
FROM node:22-alpine
LABEL org.opencontainers.image.source="https://github.com/jcardonne/tullips"
WORKDIR /app
ENV NODE_ENV=production PORT=3000 HOST=0.0.0.0
ARG RELEASE_VERSION=dev
ENV RELEASE_VERSION=$RELEASE_VERSION
COPY --from=build /app/.output ./.output
COPY --from=build /app/node_modules ./node_modules
COPY --from=build /app/package.json ./package.json
COPY --from=build /app/scripts ./scripts
COPY --from=build /app/src/lib/auth.ts ./src/lib/auth.ts
EXPOSE 3000
CMD ["npm", "start"]
