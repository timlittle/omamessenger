# Changelog

## [0.3.5](https://github.com/timlittle/omamessenger/compare/v0.3.4...v0.3.5) (2026-10-23)


### Features

* **backend:** let the helper edit your own sent text messages ([424787b](https://github.com/timlittle/omamessenger/commit/424787b02b32f4cad244789f4fe0d64c536ec76f))
* **backend:** mark a video as a silent, looping GIF and add media.save ([8557cb8](https://github.com/timlittle/omamessenger/commit/8557cb8706c1404cb6db2cf447249efe1dccf4aa))
* block a contact and report a chat as spam ([2c36354](https://github.com/timlittle/omamessenger/commit/2c36354eeeec1e73fc148975ad0fc8f63bfb5adb)), closes [#31](https://github.com/timlittle/omamessenger/issues/31)
* let the user give each account its own display name ([ab34ce9](https://github.com/timlittle/omamessenger/commit/ab34ce9aa113ba907155e8efe0ac63e063f437ac)), closes [#12](https://github.com/timlittle/omamessenger/issues/12)
* **ui:** add an in-app "What's new" list and update hint ([5ad7e90](https://github.com/timlittle/omamessenger/commit/5ad7e90cf025c898d89f79a1ec82829f3032e842))
* **ui:** edit your own sent messages from the keyboard ([a1cf74b](https://github.com/timlittle/omamessenger/commit/a1cf74bef4eb37e25ef29e7149edd94fddf0b2b1)), closes [#20](https://github.com/timlittle/omamessenger/issues/20)
* **ui:** play GIFs and videos inline, with copy, save and open actions ([90bb95e](https://github.com/timlittle/omamessenger/commit/90bb95eee383a225ddbc617da4808a5e1a438f24)), closes [#30](https://github.com/timlittle/omamessenger/issues/30)
* **ui:** show confirmations as a toast ([6367d64](https://github.com/timlittle/omamessenger/commit/6367d64679b019936f6397e08ea7fd21427abcca))


### Bug Fixes

* **backend:** recognize a real GIF image attached as a file ([e4cc49a](https://github.com/timlittle/omamessenger/commit/e4cc49abf30269f011ca8c623950be2eeba33a35))
* mark a chat read again when the window regains focus or the helper restarts ([c04cdad](https://github.com/timlittle/omamessenger/commit/c04cdadacf51d721a9ffb15a150388c832820f3d)), closes [#32](https://github.com/timlittle/omamessenger/issues/32)
* **security:** add media.copy so the helper runs wl-copy, not QML ([ad8b754](https://github.com/timlittle/omamessenger/commit/ad8b754eab7dda11813754d67495c191c6893fb2))
* **security:** always send auto for a visibility-triggered media fetch ([d91f348](https://github.com/timlittle/omamessenger/commit/d91f348f906731c3a274d429fe5fd265240a6ded))
* **security:** ask before opening a link whose text does not match its target ([1f3e62b](https://github.com/timlittle/omamessenger/commit/1f3e62b5485d56307ca4481622a34362546eb2fc))
* **security:** ask before opening a received file that can run code ([51722c2](https://github.com/timlittle/omamessenger/commit/51722c28ad071bb9e6c0a48ea8855c030e4e88f2))
* **security:** bound an animated GIF's total decoding work ([4e70947](https://github.com/timlittle/omamessenger/commit/4e709472839f922c259b175571b3856536a0ed5c))
* **security:** bound every media download by size, deadline and auto cap ([3ed8e70](https://github.com/timlittle/omamessenger/commit/3ed8e703e661aadc56f92eb9f72725853771653b))
* **security:** bound media downloads in memory before the size cap ([6094b47](https://github.com/timlittle/omamessenger/commit/6094b47a3de391a4243ea06a29abc6a378d3cb48))
* **security:** cap a conversation row's preview before laying it out ([819072c](https://github.com/timlittle/omamessenger/commit/819072c4402421422f69631cbb6a5d4d794bcb4f))
* **security:** cap a reaction's, poll's and display name's own size at ingest ([16245e3](https://github.com/timlittle/omamessenger/commit/16245e30267adb11e5600bb7497dca4fd17c9971))
* **security:** cap and sanitize clipboard and outgoing images ([6dfef07](https://github.com/timlittle/omamessenger/commit/6dfef079e6a43a7255ebf3649c74346af51869ce))
* **security:** cap concurrent notifications and bound their text ([0f572eb](https://github.com/timlittle/omamessenger/commit/0f572ebc7773c26e2999bc23f011bdb7d6d17f6a))
* **security:** cap formatting spans, mentions, reactions and poll options at ingest ([80e7e2a](https://github.com/timlittle/omamessenger/commit/80e7e2a1c7e77ccb1a2dc2e5983128eb5ae1d9d3))
* **security:** cap incoming message text and rendered table size ([f38fc42](https://github.com/timlittle/omamessenger/commit/f38fc4278242f6a861a205e6863541413d05f44a))
* **security:** destroy the search field and composer input when hidden ([de0ea17](https://github.com/timlittle/omamessenger/commit/de0ea17c83201c9af50b7ee55b637bf6982aeb65))
* **security:** escape markup characters in a notification body ([e2a6667](https://github.com/timlittle/omamessenger/commit/e2a66674a70d1e07e086052c403e7dceb3a1c1df))
* **security:** filter every link's URL scheme, in the helper and the UI ([a1574d2](https://github.com/timlittle/omamessenger/commit/a1574d296c0d35301a606b21d8bfe91fea44e145))
* **security:** keep a chat's title out of the message box's hint ([0d1d4c5](https://github.com/timlittle/omamessenger/commit/0d1d4c5cbacb9f82e3f5a5c79bb78b99b3dece96))
* **security:** keep a file's risky verdict when its chat changes mid-download ([b10d8f2](https://github.com/timlittle/omamessenger/commit/b10d8f208d17c86701a439c60d414b0fd69d347c))
* **security:** keep a JPEG's Adobe colour segment when sanitizing it ([2d8b50d](https://github.com/timlittle/omamessenger/commit/2d8b50de5fedfbc8d6941069437cfa565a9aed22))
* **security:** name cached media by id and sniffed content, never the sender's file name ([2be268a](https://github.com/timlittle/omamessenger/commit/2be268a5448ce4b042aeaf170e4b2b3d91dcdd45))
* **security:** never format a recovered panic value with anything but %T ([1466130](https://github.com/timlittle/omamessenger/commit/1466130820be88f83f51b4c44f603446f14d430f))
* **security:** never hand a video's file to the viewer's image decoder ([03cedc3](https://github.com/timlittle/omamessenger/commit/03cedc306c3b6d82108ac882bc3b62060d495a4d))
* **security:** never open a link preview's hidden URL without asking ([9e67be5](https://github.com/timlittle/omamessenger/commit/9e67be5b5a5bd9ffd57936c2ab335fbe4c0451ea))
* **security:** never put a raw sign-in error in a user-facing hint ([c37b87f](https://github.com/timlittle/omamessenger/commit/c37b87f518848d2afc2bca3a04c98b7f003676b3))
* **security:** percent-encode every local file:// URL this UI builds ([339048d](https://github.com/timlittle/omamessenger/commit/339048dd054d87ca1370690ac4cad25ba2577490))
* **security:** resolve a clicked link by its anchor, never by matching its URL ([ae7062b](https://github.com/timlittle/omamessenger/commit/ae7062baf18d28ed654ba69dc95cec4dd7f5df8c))
* **security:** restart a connector that panics instead of crashing the helper ([894e023](https://github.com/timlittle/omamessenger/commit/894e023d6e716192d6cff190a80a34799655fa44))
* **security:** route the photo viewer's external open through the one gate ([a60334b](https://github.com/timlittle/omamessenger/commit/a60334ba6269a191fa8e48998448d5d9d53f1c29))
* **security:** sanitize a picked attachment before the composer ever previews it ([645898b](https://github.com/timlittle/omamessenger/commit/645898bb60b8a4afaf78f6d4562332925ff09853))
* **security:** sanitize a saved download's file name and sniff its extension ([1bf3bca](https://github.com/timlittle/omamessenger/commit/1bf3bcacb3677c345bf06ae99d76177f5c299648))
* **security:** sanitize an outgoing photo attachment before any decode ([c535a60](https://github.com/timlittle/omamessenger/commit/c535a60a5c3f849ac7864af9f5520b2ece3ff9b8))
* **security:** sanitize every Telegram sticker download, whatever type it claims ([3427fe5](https://github.com/timlittle/omamessenger/commit/3427fe5de0ee010e0e14d4c0c4b14189f7de7f3d))
* **security:** show a fixed sentence when an account's connection fails ([016b2e3](https://github.com/timlittle/omamessenger/commit/016b2e35b5fc3369c5076b5bba5c892a9b37a22e))
* **security:** show no sender or text in notifications by default ([d609fef](https://github.com/timlittle/omamessenger/commit/d609fef74c1d0747814586b768af6c8ea2549557))
* **security:** stop rewriting the shared mimeinfo.cache on install ([0837e2c](https://github.com/timlittle/omamessenger/commit/0837e2c307e34a131a6946b5413a1eedcc044c90))
* **security:** stop wrapping copyToClipboard's command in an argv array ([02dc161](https://github.com/timlittle/omamessenger/commit/02dc161fc61d405050d5095e0d607df7142dcb50))
* **security:** stream an image download instead of ever buffering it unbounded ([0955801](https://github.com/timlittle/omamessenger/commit/09558019973c5f6c21a91bf0f096ed172252a552))
* **security:** strip compressed metadata and cap dimensions of received images ([5018bfb](https://github.com/timlittle/omamessenger/commit/5018bfbce39735485d63a392999f950ae8bbf17d))
* **security:** treat an attachment as unprepared when there is no outgoing area ([d150e5f](https://github.com/timlittle/omamessenger/commit/d150e5f4bf3a5748936a5923ee353660afc0b53d))
* **security:** validate a video's headers, require a click, one decoder at a time ([77b214c](https://github.com/timlittle/omamessenger/commit/77b214c91df35d8f3891273de6fd45ae99bce76a))
* **security:** validate a voice note's bytes before playing it in-window ([9f298da](https://github.com/timlittle/omamessenger/commit/9f298da34102ce1e8997c461dbf4eee895bff6eb))
* **security:** verify a video's own bytes before ever decoding it as a GIF ([ce31b7c](https://github.com/timlittle/omamessenger/commit/ce31b7c5167d5fcce171cdb32d8812e0aac6d999))
* **security:** verify the D-Bus sender a notification click or close comes from ([61887bf](https://github.com/timlittle/omamessenger/commit/61887bf5985884aa061c65b5fd71571cfccf2d6a))
* **security:** verify the helper against checksums committed in the repo ([f38da8b](https://github.com/timlittle/omamessenger/commit/f38da8bbc2242d924dd6edea5fbbb17b6ac21120))
* **store:** reclassify a stored GIF document from before the fix that made it animate ([a50bbb9](https://github.com/timlittle/omamessenger/commit/a50bbb9f10f681bc1f55982e5f9c57e5eb506b32))
* **ui:** declare the spelling underline delegate's model roles ([bfa61ef](https://github.com/timlittle/omamessenger/commit/bfa61ef76c2cb8c6672b8169c450b26f7602fed4))
* **ui:** make Up in an empty composer actually load the last message ([9bea5b3](https://github.com/timlittle/omamessenger/commit/9bea5b30f7d3ea66331c63adc44bdf5739e2e58a))
* **ui:** play a video or GIF in the chat on Enter, and make the toast bigger ([c288f99](https://github.com/timlittle/omamessenger/commit/c288f991d19810860f3fe0f614f95c2489eef9de))
* **ui:** play an ordinary audio message inline, not just a voice note ([9b298ee](https://github.com/timlittle/omamessenger/commit/9b298ee024cf60c75af63b92113de433ebe8fcef))
* **ui:** play real GIFs inline, add a full-size media viewer, and surface copy/save/open ([5e0588f](https://github.com/timlittle/omamessenger/commit/5e0588ff7332097a21e9a86b03ecd43208dde3bc))
* **ui:** rank a chat's own read-state commands first in the palette ([e3ea964](https://github.com/timlittle/omamessenger/commit/e3ea964cce923b4183f12d502d16fb4479139d42))
* **ui:** rank the command palette by word match and add "Mark as unread" ([2f6403e](https://github.com/timlittle/omamessenger/commit/2f6403e9d13abed1bad8ec58d87bfdfb28af2732)), closes [#33](https://github.com/timlittle/omamessenger/issues/33)
* **ui:** read theme colours in a way Qt 6.12 cannot shadow ([9fd473e](https://github.com/timlittle/omamessenger/commit/9fd473e0fa5f342f060bdf614b0c50c5eac4afa0)), closes [#29](https://github.com/timlittle/omamessenger/issues/29)
* **ui:** revalidate a cached media path's verdicts before playing it ([594c3c3](https://github.com/timlittle/omamessenger/commit/594c3c384085cf4cbd5e358b139c99473de8b3ce))
* **ui:** show a blocked contact's label in the conversation list ([9317e9a](https://github.com/timlittle/omamessenger/commit/9317e9aedb604188c944035dba9a9ed57b845d3f))
* **ui:** show the toast bottom right and make What's new larger ([16572a3](https://github.com/timlittle/omamessenger/commit/16572a3e55278a4449506fb249e4a8ae34c36066))
* **whatsapp:** open a media download's file for reading as well as writing ([5114d91](https://github.com/timlittle/omamessenger/commit/5114d91f1cd0283d50e5fe5201d54c8d688acc2f))
* **whatsapp:** save a wrapped message's media so it can be downloaded ([012173f](https://github.com/timlittle/omamessenger/commit/012173f7242981e31001191d6c080938dd11c088))

## [0.3.4](https://github.com/timlittle/omamessenger/compare/v0.3.3...v0.3.4) (2026-10-18)


### Features

* show attachment size, length and dimensions ([fe198f3](https://github.com/timlittle/omamessenger/commit/fe198f31b4b59debe58f1853d82418b0d33b884e)), closes [#24](https://github.com/timlittle/omamessenger/issues/24)
* **spell:** add English spell check to the helper ([6a943bb](https://github.com/timlittle/omamessenger/commit/6a943bb968e11332d1b3ea05025047e55293c01d))
* **ui:** show unread counts per service in the bar tooltip ([5a25c09](https://github.com/timlittle/omamessenger/commit/5a25c09c49104839414288d718b41ef44b996e44)), closes [#18](https://github.com/timlittle/omamessenger/issues/18)
* **ui:** underline misspelled words in the composer ([f646c98](https://github.com/timlittle/omamessenger/commit/f646c988f5a0c03c0548be8398b72fea9f3ba457)), closes [#13](https://github.com/timlittle/omamessenger/issues/13)


### Bug Fixes

* **cache:** refresh an adopted file's mtime so it is not pruned as stale ([2e9d19e](https://github.com/timlittle/omamessenger/commit/2e9d19e61f0d50682e69b3387f71c1a9b9d58604))
* **telegram:** keep message links and formatting ([386d471](https://github.com/timlittle/omamessenger/commit/386d47189e02151c883ce394b3753a9d5872e8a0)), closes [#26](https://github.com/timlittle/omamessenger/issues/26)
* **telegram:** retry a failed password-hint fetch during sign-in ([7bfac63](https://github.com/timlittle/omamessenger/commit/7bfac63f233724c8bdfc2c080da2cf0d1b089883)), closes [#8](https://github.com/timlittle/omamessenger/issues/8)
* **ui:** answer every caller waiting on the same media download ([a35d80a](https://github.com/timlittle/omamessenger/commit/a35d80af010ba75d686480a8f2ab596c4fac5f76))
* **ui:** destroy the command palette, account setup and new-chat dialog on close ([1c8a2a1](https://github.com/timlittle/omamessenger/commit/1c8a2a1318b39b88347b8ee16941ac28f8e2b691))
* **ui:** ignore repeat opens of the same video for a few seconds ([ae1a720](https://github.com/timlittle/omamessenger/commit/ae1a72087bc2fa5b85f27eb72b37d281edcbecbb)), closes [#22](https://github.com/timlittle/omamessenger/issues/22)
* **ui:** keep the bar tooltip text inside its bubble ([4a1e2ec](https://github.com/timlittle/omamessenger/commit/4a1e2ec88257b28f825fb87ae1102c264cbf706e)), closes [#17](https://github.com/timlittle/omamessenger/issues/17)
* **ui:** let helper errors clear themselves and be dismissed ([378c4f0](https://github.com/timlittle/omamessenger/commit/378c4f005f58784facd7f908581b1a1c1e0cf72c))
* **ui:** open a video once however many times it is clicked while loading ([332db31](https://github.com/timlittle/omamessenger/commit/332db315aea0a5df1ca08ce0806dcde6edda4514)), closes [#22](https://github.com/timlittle/omamessenger/issues/22)
* **ui:** report hover to the bar so its tooltip shows ([3ed8db0](https://github.com/timlittle/omamessenger/commit/3ed8db03d2f22d611d095554ebce0017f96cf68f)), closes [#17](https://github.com/timlittle/omamessenger/issues/17)
* **ui:** resolve a sent photo's path again once it is retired to the cache ([5a26eda](https://github.com/timlittle/omamessenger/commit/5a26edabb9e06ba6d9c7a49f6926c00f84927980)), closes [#25](https://github.com/timlittle/omamessenger/issues/25)
* **whatsapp:** address a migrated chat's mark-read patch by its LID ([4b2229a](https://github.com/timlittle/omamessenger/commit/4b2229a2739f57f496f3a7c27c1c7f12a0620bf3))
* **whatsapp:** retry a decrypt failure through the primary phone too ([05c3394](https://github.com/timlittle/omamessenger/commit/05c339437c3e95ada221b58a9d6f653cd2b2bd2b))
* **whatsapp:** save a history-synced media reference from its unwrapped message ([4b36202](https://github.com/timlittle/omamessenger/commit/4b362028b689ed8d97734c7464b7922775cdb3c0))

## [0.3.3](https://github.com/timlittle/omamessenger/compare/v0.3.2...v0.3.3) (2026-10-12)


### Features

* **telegram:** show the two-step verification password hint ([f979bfe](https://github.com/timlittle/omamessenger/commit/f979bfe0f5b64924a75d9c608e829aafb9b03cd6))


### Bug Fixes

* **doctor:** show the helper's version in the health check ([a3ba943](https://github.com/timlittle/omamessenger/commit/a3ba9430e3530e3976dc28474a75921a87e89669))
* **telegram:** resolve a reply quote's sender name instead of the generic fallback ([c7753d5](https://github.com/timlittle/omamessenger/commit/c7753d5ff5ab5252cb7869141ac2c08490ed0f61))
* **telegram:** stay on phone sign-in when the QR code refreshes ([c58933c](https://github.com/timlittle/omamessenger/commit/c58933ca94f255dd8ff04ec77c89c854218a6224))
* **ui:** make Ctrl+J insert a new line while writing again ([c2a6df7](https://github.com/timlittle/omamessenger/commit/c2a6df7649c998dae4d593b022442f9fb3b16950))
* **ui:** render text from other people as plain text so it cannot load remote images ([b8d0b1c](https://github.com/timlittle/omamessenger/commit/b8d0b1c016fea73b2c17b64386afd4de086cff46))


### Miscellaneous

* **release:** release 0.3.3 ([fbc6bff](https://github.com/timlittle/omamessenger/commit/fbc6bffe045aa082d2110da8d25e0ce0a49c985e))

## [0.3.2](https://github.com/timlittle/omamessenger/compare/v0.3.1...v0.3.2) (2026-10-09)


### Bug Fixes

* **notify:** send desktop notifications over D-Bus so message text never appears in process arguments ([f8f6aa9](https://github.com/timlittle/omamessenger/commit/f8f6aa9a9f8d0971c87ae06fe6437c5632c90f67))
* **whatsapp:** drop key-distribution-only messages instead of showing a placeholder ([52b69b2](https://github.com/timlittle/omamessenger/commit/52b69b2e61b4c1149df5160b5952af65630b06cb))

## [0.3.1](https://github.com/timlittle/omamessenger/compare/v0.3.0...v0.3.1) (2026-10-09)


### Bug Fixes

* create and keep the data directory private (0700) ([b4ced75](https://github.com/timlittle/omamessenger/commit/b4ced75b7a83adda5cc30f360ecdb60432722075))

## 0.3.0 — first release

The first tagged release: a keyboard-first unified messaging client for Omarchy, with Telegram and WhatsApp connectors.

### Features

- **Telegram sign-in and sync** — add an account by QR code or phone number and code, with two-step verification support; recent chats sync on connect, live messages, sends and read receipts stay in step with the app, and a restart or a long time offline catches up on whatever happened meanwhile instead of losing it
- **WhatsApp sign-in and sync** — add an account by QR code or phone number and an 8-character code, linking this as a companion device; every chat on the phone appears, recent ones with their history, and live messages, sends, receipts, media (including albums), reactions, replies, pin and archive, business messages' own text and "Message yourself" all stay in step with the phone, including a message deleted on either side
- **Media** — send and receive photos, videos and files; photos open in an in-window viewer sized to the window, with a blurred preview while the full image downloads; paste an image from the clipboard straight into the composer; once a sent attachment is confirmed delivered, its outgoing copy moves into the media cache instead of being kept twice
- **Voice notes** — play inline in the message bubble, with a scrubber and elapsed/total time; without `qt6-multimedia` installed, a voice note opens in your default player instead
- **Replies** — reply to any message, with a quote shown in the bubble that scrolls to the original when clicked
- **Reactions** — react to a message with a short emoji picker; one reaction per person, kept in step with Telegram and WhatsApp
- **Stickers and polls** — a sticker shows as a static image even when it is animated; a poll shows live results and can be voted on with the mouse or the keyboard (**v**); sending either is not yet supported
- **@-mentions** — typing "@" in a group conversation's composer opens a picker of its members; a message that mentions the signed-in account shows with a highlight
- **Deleting a message** — delete a message yourself (**d**), for everyone or just for you, including a failed send that never reached the service; a deletion made elsewhere syncs in too
- **Failed sends retry themselves** — a message that failed to send retries automatically once its account reconnects, and otherwise on a backoff, re-copying its attachment from the original file if the outgoing copy goes missing; a scheduled retry shows quietly beside "Not sent" as "Retrying in N min" or the time it will happen; **t** still retries sooner and starts the backoff over
- **Search** — **Ctrl+K** and **Ctrl+G** open the same palette: full-text search across every conversation, including archived ones, matching a chat's title as substring text and message bodies word by word as a prefix, ignoring case and accents
- **Incognito read receipts** — turn read receipts off from the command palette or the plugin's own setting, so a chat read here still clears locally but the service, and this account's other devices, keep showing it unread; a quiet "Read receipts off" label shows in the footer while it is on
- **Pin and archive** — pin or unpin, and archive or unarchive, a conversation, kept in step with the service; **a** archives and marks a conversation read in one step, and "Archive all read conversations" (command palette) archives every read, unpinned conversation in the current list after confirming
- **Snooze and reminders** — snooze a conversation until later today, tomorrow, next week or a custom time; it returns to the top of the list marked "Reminder" with a desktop notification once due, local to this computer only
- **Unread view** — **Ctrl+Shift+A** shows every unread conversation across every service and account in place of the usual list, and **Ctrl+K**'s jump-to-conversation list sorts unread conversations first
- **Keyboard navigation between conversations** — **Ctrl+J** and **Alt+↑/↓** move to the next or previous unread or any conversation, from the list, an open conversation or the unread view, keeping whichever mode — writing or scrolling — you were in
- **Hide and show all** — chats with no activity for a month fold out of the list, and you can hide a chat on this computer only; **Show all** brings both back, dimmed and labelled
- **History and link previews** — older messages load as you scroll back for both Telegram and WhatsApp (WhatsApp asks your phone, so it needs to be online), and links show a preview card
- **Tables** — Markdown tables in messages render as tables
- **Writing and scrolling modes** — the composer is dimmed while you scroll and shows which keys switch between the two
- **Keyboard-centric dialogs and message actions** — adding or removing an account, and the close-window question, all work from the keyboard with a visible highlight; a highlighted message can be opened (**Enter**), replied to (**r**), reacted to (**e**), have its link opened (**o**), jump to the message it replies to (**p**), retried if it failed (**t**), voted in its poll (**v**), or deleted (**d**)
- **Group chat senders** — each sender in a group chat gets a stable colour and an initials avatar on the first message of a run, so consecutive messages from different people are easy to tell apart
- **Account colours** — once more than one account is connected, each gets a stable colour tag shown in the conversation list and the rail, so chats from different accounts are easy to tell apart
- **Notifications** — desktop notifications for new messages, with three levels of detail to choose from (name and message, name only, or nothing); clicking a notification opens its conversation; messages that arrived while the helper was not running count as unread without a burst of notifications at startup
- **Keyboard and command palette** — every action reachable from the keyboard, Slack-style shortcuts, and a command palette (**Ctrl+/**) that lists every command with its shortcut
- **Key remapping** — override any default shortcut in `keys.conf`, created and opened from the command palette's "Open key bindings file"; `make keys` prints the effective bindings outside the running shell, for a bug report
- **Health check** — `oma-messenger-service doctor`, or "Run health check" in the command palette, reports the data directory's permissions, the database, the media cache, each account's connection and notify-send availability, without exposing any of their contents
- **Light on memory** — about 120 MB in use, against about 2 GB for Telegram Desktop and WhatsApp Web running side by side (see `docs/BENCHMARK.md`)
- **Install** — opening the window downloads and verifies the matching helper release the first time it is missing or out of date, with no download at plugin load time and no click needed, and **Ctrl+R** retries a failed install; installing it also adds OmaMessenger to Omarchy's apps menu

### Licensing

- Release helper binaries link go.mau.fi/libsignal (GPL-3.0) for WhatsApp's encryption, so release binaries are distributed under GPL-3.0; this repository's own source stays MIT. See the README's License section and `THIRD_PARTY_NOTICES` in each release
