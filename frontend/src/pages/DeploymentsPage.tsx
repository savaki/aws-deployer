import {createMemo, createSignal, Show} from 'solid-js'
import type {Deployment, RedeployInput} from '../components/DeploymentGrid'
import {DeploymentGrid} from '../components/DeploymentGrid'
import {createBuildsQuery, mapBuildStatus, promoteDeployment, redeployBuild} from '../lib/graphql'
import {Select, SelectItem} from '../components/ui/select'
import {showToast} from '../components/ui/toast'

interface DeploymentsPageProps {
    filterText: string
}

export function DeploymentsPage(props: DeploymentsPageProps) {
    // Environment selector (defaults to 'dev')
    const [selectedEnv, setSelectedEnv] = createSignal<'dev' | 'stg' | 'prd'>('dev')

    // Fetch builds for all environments (for desktop view)
    const devQuery = createBuildsQuery('dev')
    const stgQuery = createBuildsQuery('stg')
    const prdQuery = createBuildsQuery('prd')

    // Combine all builds from all environments
    const allBuilds = createMemo(() => {
        return [...devQuery.builds(), ...stgQuery.builds(), ...prdQuery.builds()]
    })

    // Check if any query is loading or has error
    const loading = createMemo(() => {
        return devQuery.loading() || stgQuery.loading() || prdQuery.loading()
    })

    const error = createMemo(() => {
        return devQuery.error() || stgQuery.error() || prdQuery.error()
    })

    // Transform GraphQL builds to Deployment format
    const deployments = createMemo((): Deployment[] => {
        return allBuilds().map((build) => ({
            id: build.id,
            name: build.repo,
            version: build.version,
            environment: build.env,
            status: mapBuildStatus(build.status),
            deployedAt: new Date(build.startTime),
            failureReason: build.errorMsg || undefined,
            stackName: build.stackName,
            executionArn: build.executionArn || undefined,
            downstreamEnvs: build.downstreamEnvs || [],
            deploymentErrors: build.deploymentErrors || [],
        }))
    })

    // Filter deployments based on filter text
    const filteredDeployments = createMemo((): Deployment[] => {
        const filter = props.filterText.toLowerCase().trim()
        if (!filter) {
            return deployments()
        }
        return deployments().filter(d => d.name.toLowerCase().includes(filter))
    })

    // Extract unique versions for each repo (not used for deployment history anymore)
    const versionHistory = createMemo(() => {
        const history: Record<string, string[]> = {}
        allBuilds().forEach((build) => {
            if (!history[build.repo]) {
                history[build.repo] = []
            }
            if (!history[build.repo].includes(build.version)) {
                history[build.repo].push(build.version)
            }
        })
        return history
    })

    const handleRedeploy = async (input: RedeployInput) => {
        try {
            console.log(`Redeploying ${input.name} ${input.version} to ${input.environment}`)

            await redeployBuild(input.buildId)

            showToast({
                title: 'Deployment triggered',
                description: `${input.name} ${input.version} is being deployed to ${input.environment}`,
                duration: 3000
            })

            // Refetch all queries to show the updated status
            devQuery.builds()
            stgQuery.builds()
            prdQuery.builds()
        } catch (error) {
            console.error(`Failed to redeploy ${input.name}:`, error)
            showToast({
                title: 'Deployment failed',
                description: error instanceof Error ? error.message : 'Failed to trigger deployment',
                variant: 'destructive'
            })
        }
    }

    const handlePromote = async (deployment: Deployment) => {
        try {
            console.log(`Promoting ${deployment.name} ${deployment.version} from ${deployment.environment}`)
            await promoteDeployment(deployment.id)
            console.log(`Successfully promoted ${deployment.name} ${deployment.version}`)

            // Refetch all queries to show the new promoted builds
            devQuery.builds()
            stgQuery.builds()
            prdQuery.builds()
        } catch (error) {
            console.error(`Failed to promote ${deployment.name}:`, error)
            // TODO: Show user-friendly error message
        }
    }

    return (
        <>
            {/* Environment selector - visible on mobile, hidden on desktop */}
            <div class="mb-6 mobile-env-selector items-center gap-2">
                <label class="text-sm font-medium">Environment:</label>
                <Select
                    value={selectedEnv()}
                    onValueChange={(val) => setSelectedEnv(val as 'dev' | 'stg' | 'prd')}
                    placeholder="Select environment"
                >
                    <SelectItem value="dev" onSelect={() => setSelectedEnv('dev')}>dev</SelectItem>
                    <SelectItem value="stg" onSelect={() => setSelectedEnv('stg')}>stg</SelectItem>
                    <SelectItem value="prd" onSelect={() => setSelectedEnv('prd')}>prd</SelectItem>
                </Select>
            </div>

            <Show
                when={!loading() && !error()}
                fallback={
                    <div class="text-center py-8">
                        <Show when={loading()}>
                            <p class="text-muted-foreground">Loading builds...</p>
                        </Show>
                        <Show when={error()}>
                            <p class="text-red-500">Error loading builds: {error()?.toString()}</p>
                        </Show>
                    </div>
                }
            >
                <DeploymentGrid
                    deployments={filteredDeployments()}
                    versionHistory={versionHistory()}
                    onRedeploy={handleRedeploy}
                    onPromote={handlePromote}
                    selectedEnv={selectedEnv()}
                />
            </Show>
        </>
    )
}
