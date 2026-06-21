import { parseWebViewDevtoolsSocket } from "./smoke-android-webview-login.mjs";

function assertEqual(actual, expected, message) {
  if (actual !== expected) {
    throw new Error(`${message}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`);
  }
}

const procNetUnix = [
  "Num       RefCount Protocol Flags    Type St Inode Path",
  "00000000: 00000002 00000000 00010000 0001 01 12345 /dev/socket/logdw",
  "00000000: 00000002 00000000 00010000 0001 01 23456 @webview_devtools_remote_31841",
  "00000000: 00000002 00000000 00010000 0001 01 34567 @chrome_devtools_remote"
].join("\n");

const multipleWebViews = [
  "Num       RefCount Protocol Flags    Type St Inode Path",
  "00000000: 00000002 00000000 00010000 0001 01 23456 @webview_devtools_remote_111",
  "00000000: 00000002 00000000 00010000 0001 01 23457 @webview_devtools_remote_222"
].join("\r\n");

assertEqual(
  parseWebViewDevtoolsSocket(procNetUnix),
  "webview_devtools_remote_31841",
  "failed to parse abstract WebView devtools socket"
);
assertEqual(
  parseWebViewDevtoolsSocket(multipleWebViews),
  "webview_devtools_remote_111",
  "failed to choose first WebView devtools socket"
);
assertEqual(
  parseWebViewDevtoolsSocket("Num RefCount Protocol Flags Type St Inode Path\n@chrome_devtools_remote\n"),
  "",
  "non-WebView devtools sockets must be ignored"
);
assertEqual(
  parseWebViewDevtoolsSocket(""),
  "",
  "empty adb output must not produce a socket"
);

console.log("Android WebView devtools parser ok");
