FROM node:24-alpine

# Install Gemini CLI globally
RUN npm install -g @google/gemini-cli

# Set working directory and ensure it exists with proper permissions
WORKDIR /app
RUN mkdir -p /app/tmp && \
    chown -R node:node /app && \
    chmod 755 /app

# Use the built-in node user instead of creating a new one
USER node

# Keep container running
CMD ["sh"]