# iPhone application

The iOS application bundles the existing React/MUI/Redux frontend inside Capacitor's WKWebView. It does not load a remote website as its interface. The Go API and database remain on a server. The native project is `ios/App/App.xcodeproj`, bundle identifier `com.domyshev.dwpractice`, display name `dw`, minimum iOS 15, iPhone only.

## Local simulator workflow

Requirements: macOS, Xcode with an iOS simulator runtime, Node 22 or later and Go. Run `npm ci` first.

1. Keep the mock Go API running on port 8081. For the standard local web origin: `MOCK_APP_ORIGIN=http://127.0.0.1:5173 npm run mock:api`. If it is already running, reuse it.
2. Run `npm run ios:mock`. This builds and bundles a disposable demo frontend with the native API at `http://127.0.0.1:8081`.
3. Run `npm run ios:build` to compile an unsigned simulator application.
4. Start an iPhone in Simulator (Device Hub in Xcode 27), then run `npm run ios:run`. With multiple simulators, set `IOS_SIMULATOR_UDID` explicitly.
5. Sign in with `demo@dwpractice.test` and any six-digit code. The seeded 1,014 records come from the same Go HTTP API as the web app.

Simulator loopback reaches the Mac. Loopback on a physical iPhone refers to that phone, so use a deployed HTTPS backend for device testing.

The CLI build uses Xcode's non-Keychain package authorization provider and disables Git credential-helper lookup for that process. It first refuses to run if `.netrc` or `_netrc` exists in the home directory, to avoid reading stored credentials. This does not change global Git/Xcode settings. Developers with a credential file can build in Xcode using their own authentication settings. No Mac password is needed to download the public dependencies.

## Production and physical devices

Run `VITE_NATIVE_API_URL=https://your-server.example npm run ios:prepare`, then `npm run ios:open`. Configure a unique bundle identifier if needed, select your Apple signing team, choose an attached iPhone and run from Xcode. The API URL is public build configuration, never a secret.

The API address must be an HTTPS origin without a path or embedded credentials. `VITE_NATIVE_REQUEST_ORIGIN` defaults to that origin; set it to the backend's configured `APP_ORIGIN` only when the API and web interface use different hosts. Browser requests remain same-origin with no new CORS exceptions. Native requests use URLSession through CapacitorHttp, including the backend's existing origin check, session cookie and idempotency headers. HttpOnly cookies stay in native cookie storage; no session token is copied to localStorage.

Native HTTP cannot be canceled once passed to URLSession. The existing session-generation guards and abort signals discard obsolete responses before Redux receives them. The native request has connection/read timeouts and does not follow redirects.

Release builds reject missing/incorrect build metadata and mock bundles. Prepare again with the production command before archiving. A signing team, a reachable production backend, App Store Connect metadata, privacy disclosures and review are still required for distribution; this repository does not publish automatically.

## Native integration

- Safe-area insets protect the status bar, Dynamic Island and home indicator; dialogs and forms accommodate the keyboard.
- System light/dark settings continue to work; status-bar foreground colors follow the selected theme.
- TSV, CSV and XLSX exports write to the app's private cache and open the iOS share sheet, including Save to Files. External links open Safari View Controller.
- The application icon and launch screen use the existing Diamond Way emblem without changing the artwork.
- `PrivacyInfo.xcprivacy` declares file-timestamp and app-settings API reasons. It is included in the application resources.
- Generated web assets, native caches and DerivedData are ignored. Run the preparation command after changing web source or native plugins.

This version requires a server connection to read and save journal data. It does not promise offline synchronization, background uploads, notifications, widgets or Face ID. Those are separate features rather than consequences of packaging the web frontend.

## Verification recorded for this change

The unsigned Debug app compiled and ran on an iPhone 18 Pro Max simulator with iOS 27. OTP login, daily aggregates, saving 2 large malas + 1 small mala + 4 repetitions (247), refreshed totals and audit history were exercised through the native HTTP bridge. The login session survived termination and reinstall/relaunch. XLSX export opened the native share sheet with Save to Files; the cached file was read back with both worksheets and all active/audit records. The status bar was checked on the light theme. The release-validation script rejected the bundled mock build. Frontend tests and the normal production web build also passed. Physical iPhone, software-keyboard behavior, production email delivery and App Store submission remain unverified.
