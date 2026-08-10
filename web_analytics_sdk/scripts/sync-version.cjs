const fs = require('fs');
const path = require('path');

// The SDK version tracks the Notifuse release, whose single source of truth is
// the VERSION constant in config/config.go (the same value the migration system
// compares against the database).
const versionFile = path.join(__dirname, '../../config/config.go');
const packageJsonPath = path.join(__dirname, '../package.json');

/**
 * A Notifuse release is vMAJOR.minor ("38.0"), which npm refuses: it only
 * accepts semver, so `npm publish` fails outright on a two-part version. Pad
 * the patch so the package stays publishable and still maps 1:1 back to the
 * release; anything semver already accepts is left untouched.
 *
 * This is NOT the version sent on the wire. Rollup injects the raw VERSION into
 * the bundle, so the sdk_version stored on every session is the release string
 * exactly as config.go states it.
 */
function toPackageVersion(version) {
  return /^\d+\.\d+$/.test(version) ? `${version}.0` : version;
}

function readReleaseVersion() {
  const versionContent = fs.readFileSync(versionFile, 'utf-8');
  const match = versionContent.match(/VERSION\s*=\s*['"]([^'"]+)['"]/);

  if (!match) {
    throw new Error('Could not parse VERSION from config/config.go');
  }

  return match[1];
}

function sync() {
  const release = readReleaseVersion();
  const version = toPackageVersion(release);

  const packageJson = JSON.parse(fs.readFileSync(packageJsonPath, 'utf-8'));

  if (packageJson.version === version) {
    console.log(`SDK version already at ${version}`);
    return;
  }

  packageJson.version = version;
  fs.writeFileSync(packageJsonPath, JSON.stringify(packageJson, null, 2) + '\n');
  console.log(`SDK version synced to ${version} (release ${release})`);
}

if (require.main === module) {
  try {
    sync();
  } catch (error) {
    console.error(error.message);
    process.exit(1);
  }
}

module.exports = { toPackageVersion, readReleaseVersion, sync };
