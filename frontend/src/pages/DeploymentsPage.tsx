import {createMemo, createSignal, Show} from 'solid-js'
import type {Deployment} from '../components/DeploymentGrid'
import {DeploymentGrid} from '../components/DeploymentGrid'
import {createBuildsQuery, mapBuildStatus, promoteDeployment, redeployBuild} from '../lib/graphql'
import {Select, SelectItem} from '../components/ui/select'
import {showToast} from '../components/ui/toast'

export function DeploymentsPage() {
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

    const handleRedeploy = async (deployment: Deployment, version: string) => {
        try {
            console.log(`Redeploying ${deployment.name} ${version} to ${deployment.environment}`)

            // Find the build ID for the selected version
            const buildsForRepo = allBuilds().filter(
                b => b.repo === deployment.name && b.env === deployment.environment && b.version === version
            )

            if (buildsForRepo.length === 0) {
                throw new Error(`No build found for ${deployment.name} version ${version}`)
            }

            // Use the most recent build with this version
            const buildToRedeploy = buildsForRepo.sort((a, b) =>
                new Date(b.startTime).getTime() - new Date(a.startTime).getTime()
            )[0]

            await redeployBuild(buildToRedeploy.id)

            showToast({
                title: 'Deployment triggered',
                description: `${deployment.name} ${version} is being deployed to ${deployment.environment}`,
                duration: 3000
            })

            // Refetch all queries to show the updated status
            devQuery.builds()
            stgQuery.builds()
            prdQuery.builds()
        } catch (error) {
            console.error(`Failed to redeploy ${deployment.name}:`, error)
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
            <div class="mb-4">
                <div class="flex items-center justify-between gap-2 mb-1.5 flex-wrap">
                    {/* Environment selector - visible on mobile, hidden on desktop */}
                    <div class="mobile-env-selector items-center gap-2">
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
                </div>
                <p class="text-muted-foreground text-sm">
                    Monitor and manage deployment status
                </p>
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
                    deployments={deployments()}
                    versionHistory={versionHistory()}
                    onRedeploy={handleRedeploy}
                    onPromote={handlePromote}
                    selectedEnv={selectedEnv()}
                />
            </Show>
        </>
    )
}
