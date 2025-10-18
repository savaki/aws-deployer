import type {CodegenConfig} from '@graphql-codegen/cli'

const config: CodegenConfig = {
    overwrite: true,
    schema: './schema.graphqls',
    documents: 'src/**/*.{ts,tsx}',
    generates: {
        'src/generated/graphql.ts': {
            plugins: ['typescript', 'typescript-operations'],
            config: {
                scalars: {
                    DateTime: 'string',
                },
                enumsAsTypes: true,
            },
        },
    },
}

export default config
