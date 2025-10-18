# AWS Deployer Installation Guide

This guide will walk you through setting up your application to use AWS Deployer.

## Prerequisites

We assume the AWS Deployer has also already been installed into your AWS account.

## Steps

1. Build your Lambda functions and put them into a.zip file with the name, bootstrap.
2. Upload your content to s3://{S3_ARTIFACT_BUCKET}/{repository-name}/{version}/{file-name}
3. Upload cloudformation.template s3://{S3_ARTIFACT_BUCKET}/{repository-name}/{version}/cloudformation.template
4. Generate cloudformation-params.json with the required parameters and upload it to s3:
   //{S3_ARTIFACT_BUCKET}/{repository-name}/{version}/cloudformation-params.json

The suggested params are:

* Version: The version number for this deployment (e.g. 123.abcdef)
* Env: The environment name (e.g. dev, staging, prod)
* S3Prefix: The S3 path prefix (e.g. s3://{S3_ARTIFACT_BUCKET}/{repository-name}/{version}/)

5. Uploading cloudformation-params.json will trigger the deployment.
