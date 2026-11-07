# iOS step 0: "Hello World" tooling proof

Before any real iOS work, prove the whole Apple pipeline end to end with a trivial app:
create, build, run in the Simulator, run on a real iPhone, and deliver a build through
TestFlight. Every later iOS step depends on this working, and most of it is account and
signing setup that only the owner can do.

**Status:** not started. All steps are manual (Mac + Apple account). Tick items as you go.

**Out of scope here:** any Coinslot functionality, Screen Time APIs, the backend service. Those
need their own design first (see [roadmap.md](roadmap.md)). This page only proves the tooling.

> Apple's rules, prices and requirements change often. The facts below were checked on
> 2026-11-07; re-check anything marked *(verify)* on developer.apple.com when you do the step.

## Why TestFlight and not the App Store

A Hello World app would almost certainly be **rejected** by App Store Review: guideline 4.2,
"Minimum Functionality". It also isn't something to publish under the Coinslot name. TestFlight
proves the same pipeline (signing, archive, upload, processing, install from Apple's servers)
without a public release.
- **Internal testers** (people on your App Store Connect team) don't need Beta App Review.
- The later App Store submission is the same pipeline plus store metadata, and is deferred to
  when there's a real app.

## 0. Decisions (owner)

- [ ] **Bundle ID.** Reverse-DNS of a domain you control, e.g. `com.<yourdomain>.coinslot`. It
      can't be changed once used in App Store Connect. Record it here: `________________`
- [ ] **Account type.** See §2. A paid membership is needed for TestFlight and for the Screen
      Time (Family Controls) APIs later.

## 1. Check what you have (owner, about 10 minutes)

On the Mac, in Terminal:

```sh
sw_vers                    # macOS version
xcodebuild -version        # Xcode version
xcode-select -p            # should print a path inside Xcode.app
xcrun simctl list devices available | head -20   # Simulator devices
```

- [ ] **Xcode 26 or later.** App Store Connect has rejected uploads built with older Xcode / SDK
      since 2026-04-28 *(verify: developer.apple.com/news/upcoming-requirements)*. If Xcode is
      older, update it from the Mac App Store. That may first need a newer macOS.
- [ ] Command-line tools point to Xcode. If `xcode-select -p` doesn't show `Xcode.app`, run
      `sudo xcode-select -s /Applications/Xcode.app/Contents/Developer`.
- [ ] At least one iOS Simulator runtime is installed (Xcode → Settings → Components).
- [ ] An iPhone or iPad for device testing, with **Developer Mode** available (iOS 16+).
- [ ] Apple ID with **two-factor authentication** on.
- [ ] **Do you have a developer membership?** Sign in at
      [developer.apple.com/account](https://developer.apple.com/account) → *Membership details*.
  - A team with "Apple Developer Program" and an expiry date: paid membership. Skip to §3.
  - Only "Apple Developer" / agreement acceptance, or no membership: free account. See §2.
  - In Xcode → Settings → Accounts, a team named "<Your Name> (Personal Team)" is a free account.

## 2. Account path (owner)

| | Free Apple ID ("Personal Team") | Apple Developer Program (paid) |
|---|---|---|
| Cost | $0 | US$99/year *(verify)* |
| Simulator | Yes | Yes |
| Run on your own iPhone | Yes; the install expires after **7 days** and must be re-run from Xcode | Yes; the provisioning profile lasts about a year |
| TestFlight / App Store | **No** | Yes |
| Family Controls (Screen Time APIs) | No | Development: yes. Distribution: needs a separate Apple approval (§7) |

- [ ] If you'll go beyond this proof (Coinslot on iOS needs Family Controls), **enroll** in the
      Apple Developer Program at [developer.apple.com/programs/enroll](https://developer.apple.com/programs/enroll):
  - **Individual:** your name is shown as the seller; fastest to enroll.
  - **Organization:** needs a legal entity and a D-U-N-S number, and takes longer.
  - Approval usually takes up to about 48 hours *(verify)*.
- [ ] After enrolling: App Store Connect → *Agreements, Tax, and Banking*. Accept the Free Apps
      agreement (paid agreements aren't needed for a free app).

You can do §4–§5 (Simulator and device) with a free account while enrollment is pending.

## 3. Create the project in the repo (owner, in Xcode)

The Xcode project must be created by Xcode, not hand-written, so this is a manual step.

1. Clone the repo on the Mac (or `git pull` an existing clone) and create a branch from `main`.
2. Xcode → *File → New → Project… → iOS → App*:
   - Product Name: `Coinslot`
   - Team: your team (Personal Team or paid)
   - Organization Identifier: the reverse-DNS prefix from §0 (e.g. `com.<yourdomain>`). The
     bundle ID becomes `com.<yourdomain>.Coinslot`.
   - Interface: **SwiftUI**. Language: **Swift**. Testing System: **Swift Testing**. Storage: **None**.
3. Save it **inside the repo's `ios/` folder**. Untick *Create Git repository* (the repo
   already exists). You get `ios/Coinslot.xcodeproj` and `ios/Coinslot/`.
4. In `ContentView.swift`, show something identifiable, e.g. `Text("Coinslot hello world")`.
5. Target *Coinslot* → *General* → Minimum Deployments: **iOS 17.0** (a placeholder; the real
   minimum gets decided with the Screen Time design).

- [ ] Project created at `ios/Coinslot.xcodeproj`, with the bundle ID recorded in §0.

## 4. Run in the Simulator (owner)

- [ ] In Xcode, pick an iPhone simulator and press **Run** (⌘R). The text appears.
- [ ] Build from the command line too. This is what CI will run later:

```sh
cd ios
xcodebuild -project Coinslot.xcodeproj -scheme Coinslot \
  -destination 'generic/platform=iOS Simulator' \
  CODE_SIGNING_ALLOWED=NO build
```

Expect `** BUILD SUCCEEDED **`.

- [ ] Run the template's tests: `xcodebuild … -destination 'platform=iOS Simulator,name=<device from simctl list>' test`.

## 5. Run on a real device (owner)

1. Connect the iPhone by cable (or the same Wi-Fi once paired). On the phone, tap **Trust This Computer**.
2. On the phone: *Settings → Privacy & Security → Developer Mode → On*. The phone restarts.
3. In Xcode, select the phone as the run destination and press **Run**.
4. Free account only: the first launch is blocked. On the phone, go to *Settings → General → VPN &
   Device Management → [your Apple ID] → Trust*, then run again.

- [ ] App runs on the device.

## 6. Deliver through TestFlight (owner; needs the paid membership)

1. [App Store Connect](https://appstoreconnect.apple.com) → *Apps → + → New App*: platform iOS,
   name (must be unique on the store; e.g. "Coinslot" or "Coinslot Dev"), primary language,
   the **bundle ID** from §0 (it appears once Xcode has registered it, or register it under
   *Certificates, Identifiers & Profiles → Identifiers*), and an SKU (e.g. `coinslot-ios`).
2. In Xcode, target *Coinslot* → *Signing & Capabilities*: *Automatically manage signing*, with
   the **paid** team selected.
3. Destination **Any iOS Device (arm64)**, then *Product → Archive*.
4. In the Organizer: *Distribute App → App Store Connect → Upload*. Accept the defaults.
5. Export compliance: the app uses no non-exempt encryption. Answer "None" when asked, or
   add `ITSAppUsesNonExemptEncryption = NO` to the target's Info so you aren't asked every time.
6. Wait for processing (minutes to about an hour). The build appears under the app's
   **TestFlight** tab.
7. TestFlight → *Internal Testing* → create a group, add yourself, and add the build.
8. On the iPhone: install **TestFlight** from the App Store, accept the invite, install, and launch.

- [ ] Build uploaded and processed.
- [ ] Installed from TestFlight and launched on the device.

## 7. Start the Family Controls request early (owner; paid membership)

Coinslot on iOS will need Apple's Screen Time frameworks (FamilyControls, DeviceActivity,
ManagedSettings).
- **Development:** the Family Controls capability can be added in Xcode for development builds.
- **TestFlight / App Store:** distributing needs the **Family Controls (Distribution)**
  entitlement, granted by Apple on request. Approval is per team (it covers all your bundle IDs).
- **Timing:** developer reports in 2026 range from a few days to over a month, sometimes with no
  status updates, and app extensions are sometimes approved separately from the main app. This
  is the longest lead time on the iOS path, so request it early, even before the iOS design is final.

- [ ] Request submitted (Apple Developer → Contact / Requests → Family Controls (Distribution))
      *(verify the current form location in the
      [FamilyControls documentation](https://developer.apple.com/documentation/familycontrols))*.
      Request date: `__________`

## 8. Commit and report back

- [ ] Commit `ios/Coinslot.xcodeproj` (including `project.pbxproj`) and `ios/Coinslot/`. The
      repo's `.gitignore` already excludes Xcode per-user and build files. Check `git status`
      for any `xcuserdata`, `DerivedData` or `.DS_Store` before committing.
- [ ] Push the branch and open a PR. The agent then adds the macOS CI job (§9) to the same PR.
- [ ] Send back:
  - the output of the §1 commands;
  - the account type;
  - the bundle ID;
  - which of §4–§7 worked;
  - any errors, copied exactly.

## 9. Agent follow-ups (after the owner's PR)

- Add a `macos` job to `.github/workflows/ci.yml` that runs the §4 `xcodebuild build` and
  `test` on a GitHub macOS runner with Xcode 26+. No signing in CI.
- Record the iOS stack decision as an ADR (native Swift/SwiftUI. The Screen Time frameworks and
  their extensions are Swift/Objective-C only, so the Go core can't be reused on iOS
  directly).
- Write the iOS feasibility analysis (`docs/analysis/`): what Screen Time APIs can measure
  (app usage, not active play), and limits on reporting data off the device.

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| "No account for team" / signing errors | Xcode → Settings → Accounts: add the Apple ID; select the team on the target |
| "Untrusted Developer" on the phone | Free account: trust it under VPN & Device Management (§5) |
| App stops opening after a week | Free account's 7-day profile expired; run again from Xcode |
| Upload rejected for SDK version | Xcode older than 26 (§1) |
| Bundle ID "not available" | Already registered by someone else; pick another in §0 |
| Developer Mode option missing | Connect the phone to Xcode once; the option then appears |
