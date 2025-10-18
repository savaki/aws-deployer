import {createResource, createSignal, Show} from 'solid-js'
import {IoAlertCircle, IoArrowUp, IoCopy, IoEllipsisHorizontal, IoOpenOutline, IoRocket} from 'solid-icons/io'
import {DropdownMenu} from '@kobalte/core/dropdown-menu'
import {Button} from './ui/button'
import {Badge} from './ui/badge'
import {Card, CardContent, CardFooter} from './ui/card'
import {Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle} from './ui/dialog'
import {formatDistanceToNow} from '../lib/date'
import {extractRegionFromArn, getCloudFormationUrl, getStepFunctionsUrl} from '../lib/aws'
import {fetchBuildsByRepo, mapBuildStatus} from '../lib/graphql'
import {showToast} from './ui/toast'

export type DeploymentStatus = 'success' | 'failed' | 'in-progress' | 'pending'

export interface DeploymentHistory {
    version: string
    deployedAt: Date
    status: DeploymentStatus
}

export interface DeploymentError {
    accountId: string
    region: string
    statusReason?: string | null
    stackEvents: string[]
}

interface DeploymentCardProps {
    version?: string
    status?: DeploymentStatus
    deployedAt?: Date
    failureReason?: string
    environment: string
    buildName: string
    stackName?: string
    executionArn?: string
    downstreamEnvs?: string[]
    deploymentErrors?: DeploymentError[]
    allDeployments?: Map<string, {version: string; env: string; deployedAt: Date}>
    onRedeploy: (version: string) => void
    onPromote: () => void
}

const statusConfig = {
    success: {label: 'Success', variant: 'success' as const},
    failed: {label: 'Failed', variant: 'destructive' as const},
    'in-progress': {label: 'In Progress', variant: 'warning' as const},
    pending: {label: 'Pending', variant: 'default' as const}
}

export function DeploymentCard(props: DeploymentCardProps) {
    const [isRedeployDialogOpen, setIsRedeployDialogOpen] = createSignal(false)
    const [isPromoteDialogOpen, setIsPromoteDialogOpen] = createSignal(false)
    const [isDeploymentErrorsDialogOpen, setIsDeploymentErrorsDialogOpen] = createSignal(false)
    const [selectedVersion, setSelectedVersion] = createSignal(props.version || '')

    // Fetch deployment history when dialog opens (lazy loading)
    const [deploymentHistory] = createResource(
        () => isRedeployDialogOpen() && props.buildName && props.environment,
        async () => {
            const builds = await fetchBuildsByRepo(props.buildName, props.environment)
            return builds
                .map(build => ({
                    version: build.version,
                    deployedAt: new Date(build.startTime),
                    status: mapBuildStatus(build.status),
                }))
                .sort((a, b) => b.deployedAt.getTime() - a.deployedAt.getTime())
        }
    )

    const handleRedeployClick = () => {
        setSelectedVersion(props.version || '')
        setIsRedeployDialogOpen(true)
    }

    const handleConfirmRedeploy = () => {
        props.onRedeploy(selectedVersion())
        setIsRedeployDialogOpen(false)
    }

    const handlePromoteClick = () => {
        setIsPromoteDialogOpen(true)
    }

    const handleConfirmPromote = () => {
        props.onPromote()
        setIsPromoteDialogOpen(false)
    }

    const formatDeploymentErrorsText = () => {
        let text = ''

        // Add build error if present
        if (props.failureReason) {
            text += 'Build Error:\n'
            text += props.failureReason + '\n'
            if (props.deploymentErrors && props.deploymentErrors.length > 0) {
                text += '\n---\n\n'
            }
        }

        // Add deployment errors if present
        if (props.deploymentErrors && props.deploymentErrors.length > 0) {
            if (props.failureReason) {
                text += 'Deployment Errors:\n\n'
            }
            text += props.deploymentErrors
                .map(error => {
                    let errorText = `Account: ${error.accountId}\nRegion: ${error.region}\n`
                    if (error.statusReason) {
                        errorText += `Status: ${error.statusReason}\n`
                    }
                    if (error.stackEvents && error.stackEvents.length > 0) {
                        errorText += `Events:\n${error.stackEvents.map(e => `  • ${e}`).join('\n')}\n`
                    }
                    return errorText
                })
                .join('\n---\n\n')
        }

        return text
    }

    const handleCopyErrors = async () => {
        const errorText = formatDeploymentErrorsText()
        try {
            await navigator.clipboard.writeText(errorText)
            showToast({
                title: 'Copied to clipboard',
                description: 'Deployment errors have been copied',
                duration: 3000
            })
        } catch (err) {
            showToast({
                title: 'Failed to copy',
                description: 'Could not copy to clipboard',
                variant: 'destructive'
            })
        }
    }

    if (!props.version || !props.status || !props.deployedAt) {
        return (
            <Card class="h-full bg-muted/30 border-dashed">
                <CardContent class="flex items-center justify-center h-32 p-2.5">
                    <p class="text-sm text-muted-foreground">No deployment</p>
                </CardContent>
            </Card>
        )
    }

    const config = statusConfig[props.status]

    const cardClassName = () =>
        props.status === 'failed'
            ? 'h-full hover:shadow-md transition-shadow border-destructive/50 bg-destructive/5 flex flex-col'
            : props.status === 'in-progress'
                ? 'h-full hover:shadow-md transition-shadow border-warning/50 bg-warning/5 flex flex-col'
                : 'h-full hover:shadow-md transition-shadow border-success/50 bg-success/5 flex flex-col'

    const [showPromoteTooltip, setShowPromoteTooltip] = createSignal(false)

    // Determine if promote button should be shown
    const canPromote = () => {
        // Don't show for production
        if (props.environment === 'prd') return false

        // Don't show if no downstream environments configured
        if (!props.downstreamEnvs || props.downstreamEnvs.length === 0) return false

        // Show if at least one downstream environment has a different version
        if (props.allDeployments && props.version) {
            const hasDifferentVersion = props.downstreamEnvs.some(downstreamEnv => {
                const key = `${props.buildName}-${downstreamEnv}`
                const downstreamDeployment = props.allDeployments?.get(key)
                // Show promote if downstream doesn't exist or has a different version
                return !downstreamDeployment || downstreamDeployment.version !== props.version
            })
            if (!hasDifferentVersion) return false
        }

        return true
    }

    // Get the list of downstream envs for tooltip
    const promoteTooltipText = () => {
        if (!props.downstreamEnvs || props.downstreamEnvs.length === 0) return ''
        return `Promote to ${props.downstreamEnvs.join(', ')}`
    }

    return (
        <>
            <Card class={cardClassName()}>
                <CardContent class="p-2 space-y-1 flex-1 min-h-20">
                    <div class="flex items-start justify-between">
                        <code class="text-sm bg-muted px-1 py-0.5 rounded font-mono font-medium">
                            {props.version}
                        </code>
                        <Badge variant={config.variant} class="text-xs">
                            {config.label}
                        </Badge>
                    </div>

                    <div class="text-xs text-muted-foreground">
                        {formatDistanceToNow(props.deployedAt)}
                    </div>

                    <Show when={props.status === 'failed' && (props.failureReason || (props.deploymentErrors && props.deploymentErrors.length > 0))}>
                        <div
                            class="flex items-center gap-1 text-xs text-destructive cursor-pointer hover:underline"
                            onClick={() => setIsDeploymentErrorsDialogOpen(true)}
                        >
                            <IoAlertCircle class="h-3 w-3 flex-shrink-0"/>
                            <span class="truncate">
                                <Show when={props.failureReason && props.deploymentErrors && props.deploymentErrors.length > 0}>
                                    Build and deployment errors
                                </Show>
                                <Show when={props.failureReason && (!props.deploymentErrors || props.deploymentErrors.length === 0)}>
                                    Build failed
                                </Show>
                                <Show when={!props.failureReason && props.deploymentErrors && props.deploymentErrors.length > 0}>
                                    {props.deploymentErrors!.length} deployment error{props.deploymentErrors!.length > 1 ? 's' : ''}
                                </Show>
                            </span>
                        </div>
                    </Show>
                </CardContent>

                <CardFooter class="px-2 pb-2 pt-0 flex gap-1.5">
                    <Show when={canPromote()}>
                        <div
                            class="flex-1 relative"
                            onMouseEnter={() => setShowPromoteTooltip(true)}
                            onMouseLeave={() => setShowPromoteTooltip(false)}
                        >
                            <Button
                                size="sm"
                                variant="outline"
                                onClick={handlePromoteClick}
                                class="w-full h-6 text-xs"
                            >
                                <IoArrowUp class="h-3 w-3 mr-1"/>
                                Promote
                            </Button>
                            <Show when={showPromoteTooltip() && promoteTooltipText()}>
                                <div
                                    class="absolute z-50 bottom-full mb-1 left-1/2 -translate-x-1/2 px-2 py-1 rounded bg-popover border border-border text-xs text-popover-foreground shadow-md whitespace-nowrap"
                                >
                                    {promoteTooltipText()}
                                </div>
                            </Show>
                        </div>
                    </Show>
                    <div class={canPromote() ? '' : 'ml-auto'}>
                        <DropdownMenu>
                        <DropdownMenu.Trigger
                            as={(props: any) => (
                                <Button
                                    {...props}
                                    size="sm"
                                    variant="outline"
                                    class="h-6 px-2"
                                >
                                    <IoEllipsisHorizontal class="h-4 w-4"/>
                                </Button>
                            )}
                        />
                        <DropdownMenu.Portal>
                            <DropdownMenu.Content class="z-50 min-w-[12rem] overflow-hidden rounded-md border border-border bg-popover p-1 text-popover-foreground shadow-md">
                                <DropdownMenu.Item
                                    class="relative flex cursor-pointer select-none items-center rounded-sm px-2 py-1.5 text-sm outline-none transition-colors hover:bg-accent hover:text-accent-foreground focus:bg-accent focus:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50"
                                    onSelect={handleRedeployClick}
                                >
                                    <IoRocket class="h-4 w-4 mr-2"/>
                                    Deploy Version...
                                </DropdownMenu.Item>
                                <Show when={props.executionArn}>
                                    <DropdownMenu.Item
                                        as="a"
                                        href={getStepFunctionsUrl(props.executionArn!) || '#'}
                                        target="_blank"
                                        rel="noopener noreferrer"
                                        class="relative flex cursor-pointer select-none items-center justify-between rounded-sm px-2 py-1.5 text-sm outline-none transition-colors hover:bg-accent hover:text-accent-foreground focus:bg-accent focus:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50"
                                    >
                                        <span>View Step Functions</span>
                                        <IoOpenOutline class="h-4 w-4 ml-3"/>
                                    </DropdownMenu.Item>
                                </Show>
                                <Show when={props.stackName}>
                                    <DropdownMenu.Item
                                        as="a"
                                        href={getCloudFormationUrl(props.stackName!, extractRegionFromArn(props.executionArn || '') || 'us-east-1') || '#'}
                                        target="_blank"
                                        rel="noopener noreferrer"
                                        class="relative flex cursor-pointer select-none items-center justify-between rounded-sm px-2 py-1.5 text-sm outline-none transition-colors hover:bg-accent hover:text-accent-foreground focus:bg-accent focus:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50"
                                    >
                                        <span>View CloudFormation StackSet</span>
                                        <IoOpenOutline class="h-4 w-4 ml-3"/>
                                    </DropdownMenu.Item>
                                </Show>
                            </DropdownMenu.Content>
                        </DropdownMenu.Portal>
                    </DropdownMenu>
                    </div>
                </CardFooter>
            </Card>

            <Dialog open={isRedeployDialogOpen()} onOpenChange={setIsRedeployDialogOpen}>
                <DialogContent class="max-w-md mx-4">
                    <DialogHeader>
                        <DialogTitle>Deploy to {props.environment}</DialogTitle>
                        <DialogDescription>
                            Select a version of {props.buildName} to deploy to {props.environment}
                        </DialogDescription>
                    </DialogHeader>

                    <div class="py-4 max-h-[400px] overflow-y-auto">
                        <Show
                            when={!deploymentHistory.loading && deploymentHistory()}
                            fallback={
                                <div class="text-center py-8 text-sm text-muted-foreground">
                                    Loading deployment history...
                                </div>
                            }
                        >
                            <div class="space-y-2">
                                {deploymentHistory()?.slice(0, 10).map((deployment) => (
                                    <div
                                        onClick={() => setSelectedVersion(deployment.version)}
                                        class={`p-3 rounded-md border cursor-pointer transition-colors ${
                                            selectedVersion() === deployment.version
                                                ? 'border-primary bg-primary/5'
                                                : 'border-border hover:border-primary/50 hover:bg-accent/50'
                                        }`}
                                    >
                                        <div class="flex items-center justify-between mb-1">
                                            <code class="text-sm font-mono font-medium">{deployment.version}</code>
                                            <Badge variant={statusConfig[deployment.status].variant} class="text-xs">
                                                {statusConfig[deployment.status].label}
                                            </Badge>
                                        </div>
                                        <div class="text-xs text-muted-foreground">
                                            {formatDistanceToNow(deployment.deployedAt)}
                                        </div>
                                    </div>
                                ))}
                            </div>
                        </Show>
                    </div>

                    <DialogFooter>
                        <Button variant="outline" onClick={() => setIsRedeployDialogOpen(false)}>
                            Cancel
                        </Button>
                        <Button onClick={handleConfirmRedeploy} disabled={!selectedVersion()}>
                            Deploy
                        </Button>
                    </DialogFooter>
                </DialogContent>
            </Dialog>

            <Dialog open={isPromoteDialogOpen()} onOpenChange={setIsPromoteDialogOpen}>
                <DialogContent class="max-w-md mx-4">
                    <DialogHeader>
                        <DialogTitle>Promote Build</DialogTitle>
                        <DialogDescription>
                            Confirm promotion of this build to downstream environments
                        </DialogDescription>
                    </DialogHeader>

                    <div class="space-y-3 py-4">
                        <div class="space-y-1">
                            <div class="text-sm font-medium">Repository</div>
                            <div class="text-sm text-muted-foreground">{props.buildName}</div>
                        </div>
                        <div class="space-y-1">
                            <div class="text-sm font-medium">Version</div>
                            <code class="text-sm bg-muted px-1.5 py-0.5 rounded font-mono">{props.version}</code>
                        </div>
                        <div class="space-y-1">
                            <div class="text-sm font-medium">From</div>
                            <div class="text-sm text-muted-foreground">{props.environment}</div>
                        </div>
                        <div class="space-y-1">
                            <div class="text-sm font-medium">To</div>
                            <div class="text-sm font-semibold text-primary">{props.downstreamEnvs?.join(', ')}</div>
                        </div>
                    </div>

                    <DialogFooter>
                        <Button variant="outline" onClick={() => setIsPromoteDialogOpen(false)}>
                            Cancel
                        </Button>
                        <Button onClick={handleConfirmPromote}>
                            <IoArrowUp class="h-4 w-4 mr-1"/>
                            Promote
                        </Button>
                    </DialogFooter>
                </DialogContent>
            </Dialog>

            <Dialog open={isDeploymentErrorsDialogOpen()} onOpenChange={setIsDeploymentErrorsDialogOpen}>
                <DialogContent class="max-w-2xl mx-4">
                    <DialogHeader>
                        <DialogTitle>
                            <Show when={props.failureReason && props.deploymentErrors && props.deploymentErrors.length > 0}>
                                Build and Deployment Errors
                            </Show>
                            <Show when={props.failureReason && (!props.deploymentErrors || props.deploymentErrors.length === 0)}>
                                Build Error
                            </Show>
                            <Show when={!props.failureReason && props.deploymentErrors && props.deploymentErrors.length > 0}>
                                Deployment Errors
                            </Show>
                        </DialogTitle>
                        <DialogDescription>
                            Errors that occurred during build and deployment to {props.environment}
                        </DialogDescription>
                    </DialogHeader>

                    <div class="max-h-[500px] overflow-y-auto py-4">
                        <Show when={props.failureReason}>
                            <div class="mb-4">
                                <div class="font-semibold text-sm mb-2">Build Error</div>
                                <pre class="text-sm text-destructive whitespace-pre-wrap bg-destructive/5 p-3 rounded-md border border-destructive/20">{props.failureReason}</pre>
                            </div>
                        </Show>

                        <Show when={props.deploymentErrors && props.deploymentErrors.length > 0}>
                            <Show when={props.failureReason}>
                                <div class="mb-4 border-t border-border pt-4">
                                    <div class="font-semibold text-sm mb-2">Deployment Errors</div>
                                </div>
                            </Show>
                            {props.deploymentErrors?.map((error, index) => (
                                <div class={index > 0 ? 'mt-4 pt-4 border-t border-border' : ''}>
                                    <div class="font-semibold text-sm mb-2">
                                        {error.accountId} / {error.region}
                                    </div>
                                    <Show when={error.statusReason}>
                                        <div class="mb-2 text-sm text-muted-foreground">
                                            <span class="font-medium">Status:</span> {error.statusReason}
                                        </div>
                                    </Show>
                                    <Show when={error.stackEvents && error.stackEvents.length > 0}>
                                        <div class="text-sm">
                                            <div class="font-medium mb-1">Events:</div>
                                            <div class="space-y-1 pl-2">
                                                {error.stackEvents.map((event) => (
                                                    <div class="text-destructive">• {event}</div>
                                                ))}
                                            </div>
                                        </div>
                                    </Show>
                                </div>
                            ))}
                        </Show>
                    </div>

                    <DialogFooter>
                        <Button variant="outline" onClick={() => setIsDeploymentErrorsDialogOpen(false)}>
                            Close
                        </Button>
                        <Button onClick={handleCopyErrors}>
                            <IoCopy class="h-4 w-4 mr-2"/>
                            Copy Errors
                        </Button>
                    </DialogFooter>
                </DialogContent>
            </Dialog>
        </>
    )
}
