import {defineConfig} from 'vite'
import solid from 'vite-plugin-solid'

export default defineConfig({
    plugins: [solid()],
    server: {
        proxy: {
            '/graphql': {
                target: 'http://localhost:8080',
                changeOrigin: true,
            },
            '/login': {
                target: 'http://localhost:8080',
                changeOrigin: true,
            },
            '/logout': {
                target: 'http://localhost:8080',
                changeOrigin: true,
            },
            '/oauth': {
                target: 'http://localhost:8080',
                changeOrigin: true,
            },
        },
    },
})
