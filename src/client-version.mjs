// Evidence baseline, not an installation pin or a promise of compatibility.
export const REFERENCE_CLIENT_VERSION = '0.153.4';
const release = '(?:0|[1-9][0-9]{0,5})';
const versionPattern = new RegExp(`^${release}\\.${release}\\.${release}(?:-[0-9A-Za-z-]+(?:\\.[0-9A-Za-z-]+)*)?(?:\\+[0-9A-Za-z-]+(?:\\.[0-9A-Za-z-]+)*)?$`);

export function isClientVersion(value) {
  return typeof value === 'string' && value.length <= 96 && !/\s/.test(value) && versionPattern.test(value);
}

export function clientVersionPolicy(clientVersion) {
  if (!isClientVersion(clientVersion)) {
    throw Object.assign(new Error('CLI_VERSION_INVALID'), { code: 'CLI_VERSION_INVALID' });
  }
  return Object.freeze({ clientVersion, referenceClientVersion: REFERENCE_CLIENT_VERSION,
    clientVersionStatus: clientVersion === REFERENCE_CLIENT_VERSION ? 'reference' : 'unverified' });
}
