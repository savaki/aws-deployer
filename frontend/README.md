# AWS Deployer Frontend

A modern web interface for the AWS Deployer management console, built with SolidJS and Tailwind CSS.

## Tech Stack

- **SolidJS** - Reactive UI library
- **TypeScript** - Type-safe JavaScript
- **Tailwind CSS** - Utility-first CSS framework
- **Vite** - Fast build tool and dev server

## Getting Started

### Install Dependencies

```bash
npm install
```

### Development

Start the development server:

```bash
npm run dev
```

The app will be available at `http://localhost:5173`

### Build

Build for production:

```bash
npm run build
```

The built files will be in the `dist` directory.

### Preview

Preview the production build locally:

```bash
npm run preview
```

## Project Structure

```
frontend/
├── src/
│   ├── App.tsx          # Main application component
│   ├── index.css        # Tailwind CSS imports
│   └── index.tsx        # Application entry point
├── public/              # Static assets
├── index.html           # HTML template
├── tailwind.config.js   # Tailwind CSS configuration
├── postcss.config.js    # PostCSS configuration
├── tsconfig.json        # TypeScript configuration
└── vite.config.ts       # Vite configuration
```

## Features

- ✅ SolidJS for reactive UI
- ✅ TypeScript for type safety
- ✅ Tailwind CSS for styling
- ✅ Vite for fast development
- ✅ Hot Module Replacement (HMR)

## API Integration

The frontend will connect to the AWS Deployer API backend to:

- View build deployments
- Monitor CloudFormation stack status
- Manage deployment history

API endpoint configuration can be set via environment variables.
