import { collectClientBuildPrereqs } from "./client-build-env.mjs";

const result = collectClientBuildPrereqs();

console.log(JSON.stringify(result, null, 2));

if (!result.desktopArtifactReady || !result.androidApkReady) {
  process.exitCode = 2;
}
