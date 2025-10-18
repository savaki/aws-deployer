import {For, Show} from 'solid-js'
import {createPipelinesQuery, type PipelineConfig} from '../lib/graphql'
import {TbArrowRight} from 'solid-icons/tb'

export function EnvironmentsPage() {
    const query = createPipelinesQuery()

    return (
        <div class="min-h-screen bg-background">
            <div class="container mx-auto px-4 py-6 max-w-7xl">
                <div class="mb-6">
                    <h1 class="text-3xl font-bold tracking-tight mb-2">Environment Pipelines</h1>
                    <p class="text-muted-foreground">
                        Deployment promotion structure showing how builds flow through environments
                    </p>
                </div>

                <Show
                    when={!query.loading() && !query.error()}
                    fallback={
                        <div class="text-center py-8">
                            <Show when={query.loading()}>
                                <p class="text-muted-foreground">Loading pipeline configurations...</p>
                            </Show>
                            <Show when={query.error()}>
                                <p class="text-red-500">Error loading pipelines: {query.error()?.toString()}</p>
                            </Show>
                        </div>
                    }
                >
                    <div class="space-y-8">
                        <For each={query.pipelines()}>
                            {(pipeline) => <PipelineDAG pipeline={pipeline}/>}
                        </For>
                    </div>
                </Show>
            </div>
        </div>
    )
}

interface PipelineDAGProps {
    pipeline: PipelineConfig
}

function PipelineDAG(props: PipelineDAGProps) {
    const displayName = () => props.pipeline.repo === '$' ? 'Default' : props.pipeline.repo

    return (
        <div class="border border-border rounded-lg p-6 bg-card">
            <div class="mb-4">
                <h2 class="text-2xl font-semibold">{displayName()}</h2>
                <p class="text-sm text-muted-foreground">Initial environment: {props.pipeline.initialEnv}</p>
            </div>

            <div class="flex items-center gap-4 overflow-x-auto pb-4">
                <For each={props.pipeline.environments}>
                    {(env) => (
                        <>
                            <EnvironmentNode
                                env={env.env}
                                accounts={getAccounts(env.targets)}
                                regions={getRegions(env.targets)}
                            />
                            <Show when={env.downstreamEnvs.length > 0}>
                                <div class="flex items-center gap-2 text-muted-foreground">
                                    <TbArrowRight class="h-6 w-6"/>
                                    <Show when={env.downstreamEnvs.length > 1}>
                                        <span class="text-xs">({env.downstreamEnvs.join(', ')})</span>
                                    </Show>
                                </div>
                            </Show>
                        </>
                    )}
                </For>
            </div>
        </div>
    )
}

interface EnvironmentNodeProps {
    env: string
    accounts: string[]
    regions: string[]
}

function EnvironmentNode(props: EnvironmentNodeProps) {
    const envColors: Record<string, string> = {
        dev: 'bg-blue-500/10 border-blue-500',
        stg: 'bg-yellow-500/10 border-yellow-500',
        prd: 'bg-green-500/10 border-green-500',
    }

    const color = envColors[props.env] || 'bg-gray-500/10 border-gray-500'

    return (
        <div
            class={`border-2 rounded-lg p-4 min-w-[200px] ${color}`}
        >
            <div class="font-semibold text-lg mb-2">{props.env}</div>
            <div class="space-y-2 text-sm">
                <div>
                    <div class="text-muted-foreground font-medium">Accounts:</div>
                    <div class="font-mono text-xs">
                        <For each={props.accounts} fallback={<span class="text-muted-foreground">None</span>}>
                            {(account) => <div>{account}</div>}
                        </For>
                    </div>
                </div>
                <div>
                    <div class="text-muted-foreground font-medium">Regions:</div>
                    <div class="text-xs">
                        <For each={props.regions} fallback={<span class="text-muted-foreground">None</span>}>
                            {(region) => <div>{region}</div>}
                        </For>
                    </div>
                </div>
            </div>
        </div>
    )
}

// Helper functions to extract unique accounts and regions from targets
function getAccounts(targets: Array<{ accountIds: string[]; regions: string[] }>): string[] {
    const accounts = new Set<string>()
    targets.forEach(target => {
        target.accountIds.forEach(id => accounts.add(id))
    })
    return Array.from(accounts).sort()
}

function getRegions(targets: Array<{ accountIds: string[]; regions: string[] }>): string[] {
    const regions = new Set<string>()
    targets.forEach(target => {
        target.regions.forEach(region => regions.add(region))
    })
    return Array.from(regions).sort()
}
