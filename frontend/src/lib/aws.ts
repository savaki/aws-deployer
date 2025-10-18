/**
 * Extract AWS region from any ARN
 * ARN format: arn:aws:{service}:{region}:{account}:...
 */
export function extractRegionFromArn(arn: string): string | null {
    const match = arn.match(/^arn:aws:[^:]+:([^:]+):/)
    return match ? match[1] : null
}

/**
 * Generate AWS Step Functions console URL for an execution
 */
export function getStepFunctionsUrl(executionArn: string): string | null {
    const region = extractRegionFromArn(executionArn)
    if (!region) return null

    const encodedArn = encodeURIComponent(executionArn)
    return `https://console.aws.amazon.com/states/home?region=${region}#/v2/executions/details/${encodedArn}`
}

/**
 * Generate AWS CloudFormation console URL for a StackSet
 */
export function getCloudFormationUrl(stackName: string, region: string = 'us-east-1'): string {
    return `https://console.aws.amazon.com/cloudformation/home?region=${region}#/stacksets/${encodeURIComponent(stackName)}`
}
