FROM node:24-alpine

# Set working directory
WORKDIR /app

# Copy package files and install dependencies
COPY package*.json ./
RUN npm install --production

# Copy the rest of the application code
COPY . .

# Set the entrypoint for the action
ENTRYPOINT ["node", ".github/scripts/semantic-lint.js"]