package rpc

const MetadataServiceAccountNameKey = "x-orchard-service-account-name"

//nolint:gosec // G101 check yields a false-positive here, this is not a hard-coded credential
const MetadataServiceAccountTokenKey = "x-orchard-service-account-token"

const MetadataWorkerKey = "x-orchard-worker"

const MetadataWorkerPortForwardingSessionKey = "x-orchard-port-forwarding-session"
