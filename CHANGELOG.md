# Changelog

## [0.3.6](https://github.com/timlittle/omamessenger/compare/v0.3.5...v0.3.6) (2026-11-04)


### Features

* add custom notification sounds per service and per conversation ([40ef1db](https://github.com/timlittle/omamessenger/commit/40ef1dbf2d8dce092984f0eb77c4d50b0a0e577e)), closes [#19](https://github.com/timlittle/omamessenger/issues/19)
* allow pinning unlimited chats, with keyboard and drag reordering ([62e1cf4](https://github.com/timlittle/omamessenger/commit/62e1cf4c277e7bbbd785cd04d1a0caca64324bd1)), closes [#14](https://github.com/timlittle/omamessenger/issues/14)
* **attachments:** show and enforce per-account attachment limits ([71cd578](https://github.com/timlittle/omamessenger/commit/71cd578e281bb0f039a62ffa0201615ef342d044)), closes [#16](https://github.com/timlittle/omamessenger/issues/16)
* **conversations:** restore a chat's pin when it is unarchived ([10a9d82](https://github.com/timlittle/omamessenger/commit/10a9d8206ab96ca08a3a44953d6df3951d42d2c1))
* **media:** add a bubblewrap-sandboxed ffmpeg transcoder for untrusted media ([05df1ee](https://github.com/timlittle/omamessenger/commit/05df1eede577c1a033199c12eec70b33b5bbf536))
* **media:** play video, voice and audio through the sandboxed transcoder ([5c2200c](https://github.com/timlittle/omamessenger/commit/5c2200cb365fd8ed49c146c50d0401583251201d))
* schedule a message to send later ([f2efa3a](https://github.com/timlittle/omamessenger/commit/f2efa3a4cf97fe785a27fcd194bfa553b38ef7fb))
* **ui:** add 5- and 30-minute send-later presets and short-duration wording ([6316e3f](https://github.com/timlittle/omamessenger/commit/6316e3f3e88605c6c1ad13daa5c351afed06698a))
* **ui:** add a "Play notification sound" command ([97619cc](https://github.com/timlittle/omamessenger/commit/97619cc284f66e350ff6391e7bf01beb2d3b3c4d))
* **ui:** add keyboard commands for a queued message ([505e928](https://github.com/timlittle/omamessenger/commit/505e9280976b1c4f9c04a118e03cdd79bf7f3e62))
* **ui:** confirm before archiving, offer undo, and guard against mistyped shortcuts ([ead0634](https://github.com/timlittle/omamessenger/commit/ead063413e62d332bea89461d1b56a432d8157c9))
* **ui:** give send-later a real prompt and a composer entry point ([4ad863f](https://github.com/timlittle/omamessenger/commit/4ad863f8fd8c7dea3f90223eada7936c50c728b2))
* **ui:** play media only from its sandboxed play copy, never the original ([62681f8](https://github.com/timlittle/omamessenger/commit/62681f8d83f53a1374e0684c8ba84e74793f5b39))
* **ui:** reach queued messages and send later from the keyboard ([d0523bd](https://github.com/timlittle/omamessenger/commit/d0523bdba18bd1994e1f2041b696a009e0717d37))
* **ui:** reopen the window after the shell restarts if it was open ([fa796a2](https://github.com/timlittle/omamessenger/commit/fa796a2fd47cc24bc499accc076d35febcf30f8b))


### Bug Fixes

* **app:** never remove a display copy's original for an unrelated failure ([f7fd885](https://github.com/timlittle/omamessenger/commit/f7fd885da12968533c84dac3a09352c923dc16c2))
* **app:** remember a failed play copy so reopening a chat does not retranscode it ([c6bc752](https://github.com/timlittle/omamessenger/commit/c6bc7529918bafd8969352cbc12ab8c8f27e1629))
* **audiosafe:** never play MP3 in-window, only the first frames were ever checked ([064f9b3](https://github.com/timlittle/omamessenger/commit/064f9b385cfb81877fc3ad9ac1f3aee768c12b76))
* **audiosafe:** refuse any MP3 carrying an ID3v2, APE or Lyrics3 tag ([1b484c9](https://github.com/timlittle/omamessenger/commit/1b484c946c0af5554c49fe7d0c0c58a640c8ddb9))
* **backend:** cap a thumbnail's re-encoded size, not only its raw input ([6c67ec6](https://github.com/timlittle/omamessenger/commit/6c67ec6a078bce6526918007952ff2a19ec4c900))
* **backend:** carry verified through group members, contacts and matched senders ([c0c3b51](https://github.com/timlittle/omamessenger/commit/c0c3b517ff41eb1f0d05e1ba22d204b559c513bd))
* **backend:** carry whether a display name is verified through the protocol ([7917f2f](https://github.com/timlittle/omamessenger/commit/7917f2fc5b27af7477b1b4c7ee5e5d5e3c4cc257))
* **backend:** mark a conversation opened from the contact picker as verified ([cfc18d6](https://github.com/timlittle/omamessenger/commit/cfc18d68bd9069e9f3682cb55e60b62ae4da382c))
* **backend:** mark the duplicated protocol stdout descriptor close-on-exec ([c99517f](https://github.com/timlittle/omamessenger/commit/c99517ffbad379b1a3bd7dfdaa87d14b99075362))
* **backend:** refuse to store a message with an oversized remote or sender id ([c07939b](https://github.com/timlittle/omamessenger/commit/c07939b673b9a97ad59c5ab726a4b7c6be200979))
* **backend:** stop marking a notification title for an unverified sender ([9faca94](https://github.com/timlittle/omamessenger/commit/9faca949d055e72150ac4b4622c6d708f6c24afe))
* **backend:** stop the decode-budget estimate from being fooled by header tricks ([02ebd75](https://github.com/timlittle/omamessenger/commit/02ebd757a9967578d1622c84950090bb9dc718f1))
* **cache:** never serve a notification sound this helper did not itself transcode ([75364ba](https://github.com/timlittle/omamessenger/commit/75364baecbbeb31d38e0287dff1d1caeb43a3f99))
* **connector:** check a reencode source file's size before reading it ([1d5382b](https://github.com/timlittle/omamessenger/commit/1d5382b14f5bd6a92c5f03290165be3b471eb641))
* **domain:** cap a mention's and a reply quote's own id ([0670d32](https://github.com/timlittle/omamessenger/commit/0670d3216babc55de4ef73107844b7f27ad918ae))
* **domain:** cap a reply quote's own sender id like its remote id ([1a9418e](https://github.com/timlittle/omamessenger/commit/1a9418e60d362d129d43c9eeec991f51673f0839))
* **domain:** cap each mention's own Name, not just how many survive ([2355565](https://github.com/timlittle/omamessenger/commit/2355565e2e0bc6827d8761a1d04baaf815d179d2))
* **domain:** re-cap a reply quote's text on every read, not only on write ([80448d3](https://github.com/timlittle/omamessenger/commit/80448d3fed67096177aebb4760dbcf4edc5098c3))
* **domain:** strip control and bidi characters from display names ([8f8521f](https://github.com/timlittle/omamessenger/commit/8f8521fb869d9d1fd0b7cffb1ff10579d68ebf61))
* **helper:** keep stdout for protocol traffic only ([3f5bb13](https://github.com/timlittle/omamessenger/commit/3f5bb1318eaeac68b7f60e35f114ff85d1021f68))
* **helper:** use Dup3 so the arm64 helper builds ([2584f39](https://github.com/timlittle/omamessenger/commit/2584f397443512cdc0e03c3064bafbea78412e3d))
* **imagesafe:** cap a JPEG's progressive scan count before decoding it ([39b7cf8](https://github.com/timlittle/omamessenger/commit/39b7cf883016fa7afbeecdf998cc505a1a664e69))
* **imagesafe:** never trust an extended WebP's canvas for its inner chunk's own size ([e68a95b](https://github.com/timlittle/omamessenger/commit/e68a95bee3f709530a21d2fd21c3e4de86c791fd))
* **links:** refuse a URL with a backslash, hidden character, or no real authority ([140d0ad](https://github.com/timlittle/omamessenger/commit/140d0ad374959f28a52860d5db7a4e53a8666cf2))
* **media:** accept only known MP4 boxes, so a compressed moov cannot skip validation ([f36d8af](https://github.com/timlittle/omamessenger/commit/f36d8afae40d47bc33c993da2158e92f70226c41))
* **media:** bind a media.fetch answer to the kind it actually served ([085c686](https://github.com/timlittle/omamessenger/commit/085c686f54cea974e255e40f3a84156726f38153))
* **media:** bound a GIF's own frame delay to a minimum decode rate ([078847e](https://github.com/timlittle/omamessenger/commit/078847e87706fe08bc70c086071da9f691ff488f))
* **media:** bound concurrent photo decodes and stop re-transcoding an unchanged notification sound ([264912b](https://github.com/timlittle/omamessenger/commit/264912bd6a4880fc41f731ed69c39741064bd9b8))
* **media:** cap a JPEG's repeated scans, DHT and DQT segments ([876e5f2](https://github.com/timlittle/omamessenger/commit/876e5f299efc1656dc6f0e8eeac6c1b2e9e59c73))
* **media:** cap and drop oversized link URLs, fix WhatsApp preview ordering ([33110df](https://github.com/timlittle/omamessenger/commit/33110df7c25ac1cd11142a496cd2f290e12eb500))
* **media:** cap aspect ratio so a bounded sourceSize cannot still blow up ([1822c8a](https://github.com/timlittle/omamessenger/commit/1822c8a3f4a52e86bd4bf0da68fa09c092c4a2b1))
* **media:** classify a bare SVG and other active content as risky by content ([7c2c834](https://github.com/timlittle/omamessenger/commit/7c2c834a77518a006ad7d0d934b29e861c506851))
* **media:** clear lint on the track-completeness and in-band scan changes ([a739198](https://github.com/timlittle/omamessenger/commit/a7391980b14100e66dd076193ec38577825b45b0))
* **media:** decode and re-encode images in Go instead of sanitizing containers ([4c30736](https://github.com/timlittle/omamessenger/commit/4c30736245df4d00b34b382eb84bacee1be79ec4))
* **media:** give outgoing photos their own, larger profile instead of refusing them ([3392b14](https://github.com/timlittle/omamessenger/commit/3392b149fa80e1b969bad113723a143d76871f80))
* **media:** keep a photo's verified decode size across an unrelated update ([93022cd](https://github.com/timlittle/omamessenger/commit/93022cd90ca76302733c50656e1f262b32e981a9))
* **media:** only copy media whose sniffed bytes match the claimed kind ([6c69d72](https://github.com/timlittle/omamessenger/commit/6c69d7297dbcb6a707e15ec62430d9694c30165b))
* **media:** re-check images an older release saved to disk ([7e0c4aa](https://github.com/timlittle/omamessenger/commit/7e0c4aa044f1fb3b168488ce1b0626370b941534))
* **media:** re-verify every photo and sticker against imagesafe at serve time ([814825a](https://github.com/timlittle/omamessenger/commit/814825a1e6e2c80edd8f948fa996409a06ab57bc))
* **media:** refuse an MP4/Ogg track or stream neither videosafe nor audiosafe validated ([5970d3a](https://github.com/timlittle/omamessenger/commit/5970d3a20f103017772cf2601ce61a1cf0efd848))
* **media:** refuse audio and video carrying an embedded cover picture ([0a84a20](https://github.com/timlittle/omamessenger/commit/0a84a20ada3d0871860fb57a38a5112726d8b44b))
* **media:** reject a GIF frame whose rectangle exceeds its canvas ([b7bfb91](https://github.com/timlittle/omamessenger/commit/b7bfb91d44c50f39befe7024c3cf8dd2aa89a435))
* **media:** reject a symlinked transcode output, serialize jobs, and bound video bitrate ([db76dea](https://github.com/timlittle/omamessenger/commit/db76dea9e91419291d9d2808080a366f5ac1321e))
* **media:** reject GIFs whose timing block Qt could parse differently ([52545a4](https://github.com/timlittle/omamessenger/commit/52545a421b767982dac1021de0084edfb2a8f1f5))
* **media:** satisfy lint on the media-copy cap fix ([a50ea90](https://github.com/timlittle/omamessenger/commit/a50ea90010a20f2e51cfe577759bb7137b31951a))
* **media:** size a real GIF video's decode from its verified canvas, not the sender's claim ([f70e710](https://github.com/timlittle/omamessenger/commit/f70e7107943d185e5e6155f1e7c43e42225925d5))
* **media:** size image decodes from verified dimensions, not a sender's claim ([21dedd9](https://github.com/timlittle/omamessenger/commit/21dedd9631f2f49c6e8dc77b7a2bb7d22dda19d3))
* **media:** skip 0xFF fill bytes before classifying a JPEG marker ([38771fe](https://github.com/timlittle/omamessenger/commit/38771fed6420b609fd7db6832d5e6300aa8ca77a))
* **media:** strip markup characters from a saved file's own name ([a7a9f7b](https://github.com/timlittle/omamessenger/commit/a7a9f7b979843c435592aa3355db4a3c6f0375ea))
* **media:** walk the whole file for an MP4/M4A cover art atom ([cda0d24](https://github.com/timlittle/omamessenger/commit/cda0d247750406695df28349f08fd984961e2b8f))
* **sandbox:** bound the sandboxed job's thread count with an inner prlimit ([a241704](https://github.com/timlittle/omamessenger/commit/a24170415fd67567ff34ad272d0a7aba60788443))
* **sandbox:** close a seccomp bypass, block nested namespaces, and stop crash notifications ([4c835d8](https://github.com/timlittle/omamessenger/commit/4c835d87ee66e1232a3333ad688c62b6a8853c13))
* **sandbox:** deny memfd syscalls and cap /dev/shm and bwrap's own root ([171fb07](https://github.com/timlittle/omamessenger/commit/171fb0767b8af3887e29446ec9db8ff3a00f5a0b))
* **sandbox:** deny System V and POSIX IPC syscalls in the seccomp filter ([9a68a58](https://github.com/timlittle/omamessenger/commit/9a68a584c13b5da4535123f2416bc9796cf39ba7))
* **sandbox:** never let a caller's own file path reach the sandbox argv ([de6aa40](https://github.com/timlittle/omamessenger/commit/de6aa40e84d4943783ee16a4252e6871375acd5e))
* **sandbox:** refuse forking, cap /tmp, and bind output as one file ([f46b8b6](https://github.com/timlittle/omamessenger/commit/f46b8b6f050b5ec16a19778303ae79c7541a3ef9))
* **security:** cap sticker emoji, spell-check text, search snippets and stale media verdicts ([bfd897f](https://github.com/timlittle/omamessenger/commit/bfd897f20ae2968bff0f7104103072a38a3eb758))
* **spell:** cap the word length spell.suggest and spell.addWord accept ([be0cbb0](https://github.com/timlittle/omamessenger/commit/be0cbb008ec3554b4721ec2e99b31ba3a7a00621))
* **store:** cap a conversation's preview on every read and write ([73a29ff](https://github.com/timlittle/omamessenger/commit/73a29ff6907c97b434336e1f61788acdb5cadfcf))
* **store:** cap a search match snippet on read, not just preview ([6412a8c](https://github.com/timlittle/omamessenger/commit/6412a8cc5663db9ac794e5cb3237e3b5babdf10b))
* **store:** carry a contact's real verified flag and phone through ([4839839](https://github.com/timlittle/omamessenger/commit/4839839c89e42a19017f23cd0261378bb15906e3))
* **store:** give pre-existing pinned chats a distinct order to move ([d27e35f](https://github.com/timlittle/omamessenger/commit/d27e35ff772b0cd0ab6d2d19948e08361ca0ed9b))
* **store:** re-apply ingest caps to legacy rows on every read ([7611e0b](https://github.com/timlittle/omamessenger/commit/7611e0b1af3f33f56315d9de3333531991b1522f))
* **store:** sanitize every stored thumbnail, not just new ones ([4da1363](https://github.com/timlittle/omamessenger/commit/4da13634da25e327e93bdbe7680aa1d74ca4dd0a))
* **store:** scope a sender rename's reply-quote correction to its own conversation ([6436eb6](https://github.com/timlittle/omamessenger/commit/6436eb6dad5325b6d1dea8537c804ef17aa36ff7))
* **store:** verify a thumbnail's dimensions on write, not only on read ([19b87e8](https://github.com/timlittle/omamessenger/commit/19b87e89897e3de82aa020b63ef566c48977dfa2))
* **telegram:** never duplicate or strand a placeholder conversation ([46ad988](https://github.com/timlittle/omamessenger/commit/46ad9880ac2e11e5bd278d6b5c4535fec4cd8cbe))
* **telegram:** never silently drop a first message from an unknown peer ([6a5a404](https://github.com/timlittle/omamessenger/commit/6a5a404f84ccd092dd5bc888298a985e700052a6))
* **transcode:** bound the intermediate's own top-level boxes before mp4ff reads them ([d929b4c](https://github.com/timlittle/omamessenger/commit/d929b4c228b24713faaa735cb0bca7049f28614f))
* **transcode:** cap every job's own encoder, filters and scheduling priority ([0c5fd3c](https://github.com/timlittle/omamessenger/commit/0c5fd3c5ac361f85d0768d57082a21f066f988b7))
* **transcode:** cap the rebuilt MP4's total size and remember a failed sound transcode ([55a6c47](https://github.com/timlittle/omamessenger/commit/55a6c47f1ace6a5dc65092fb452602f6189dbc75))
* **transcode:** catch an SPS that crops a huge coded picture down to a small one ([cbd00f7](https://github.com/timlittle/omamessenger/commit/cbd00f7c77a34ba77069654a3ddebcac619a1543))
* **transcode:** never trust the sandboxed ffmpeg's own output file structure ([ca5ebfa](https://github.com/timlittle/omamessenger/commit/ca5ebfa62a5a35898d269ff549f79fdd44ee33a9))
* **transcode:** refuse an oversized WAV fmt chunk before allocating a buffer for it ([7c6fb9c](https://github.com/timlittle/omamessenger/commit/7c6fb9c93cbe8874a7aa76d3be071819e6138cca))
* **transcode:** remove a job's own directory on every failure path ([d47aea7](https://github.com/timlittle/omamessenger/commit/d47aea75c486273aa97ab166ceeed7a04799526e))
* **ui:** always confirm a bidi-hidden, truncated, or lookalike link ([23e1b27](https://github.com/timlittle/omamessenger/commit/23e1b27c8f690ea8046c0b440b9537806a3c4e9c))
* **ui:** bound pipe-table parsing to linear time ([35fd8f6](https://github.com/timlittle/omamessenger/commit/35fd8f6107f6b8a489c6859fcfbe29ddd25d7cb7))
* **ui:** cap the open chat's timeline even while a row is highlighted ([fe603f3](https://github.com/timlittle/omamessenger/commit/fe603f356fb88fa1a4f34ae1219b011c55430e6d))
* **ui:** cap the open conversation's timeline so a flood of messages cannot grow it, or its own per-event cost, without bound ([864a85e](https://github.com/timlittle/omamessenger/commit/864a85e2ac45602a5a93a42c987eafc91ae8e4d8))
* **ui:** cap the total pipe-table cells and tables per message ([df3b233](https://github.com/timlittle/omamessenger/commit/df3b233d8b8e55c7f0c816f18f98c076f590dcf3))
* **ui:** catch Qt's own paste shortcut ahead of every field's binding lookup ([e5cf851](https://github.com/timlittle/omamessenger/commit/e5cf8517d6987f0f345ecde8712ff70e06d355af))
* **ui:** check a link preview's own URL against the bubble's real anchors, not a text substring ([9c7e9d9](https://github.com/timlittle/omamessenger/commit/9c7e9d9ce78dfe96871d3b8c6655bd8ae1009381))
* **ui:** close the photo viewer when an edit swaps its media for a kind it cannot show ([8bc1076](https://github.com/timlittle/omamessenger/commit/8bc1076f96713ccf4c5f3b07cbdc57a485e77acc))
* **ui:** compute a message's URL-token continuation once per render ([1168d3b](https://github.com/timlittle/omamessenger/commit/1168d3ba57577678ac3a6bc8cc21c647c6af9cb8))
* **ui:** destroy account-setup and rename-account fields per step, not per overlay ([1f03e0a](https://github.com/timlittle/omamessenger/commit/1f03e0a872bd57e840d959af93b5dc09bdb74dec))
* **ui:** destroy editable fields in four overlays while the window is inactive ([9e76f6b](https://github.com/timlittle/omamessenger/commit/9e76f6b29592ded8f5cb8ba3346eb64dd1900faf))
* **ui:** destroy fallback file dialogs when they close, not just hide ([63233a5](https://github.com/timlittle/omamessenger/commit/63233a55f05a7bf591eefb12628767846ccfa1c9))
* **ui:** destroy the composer's input while the window is inactive or the chat is blocked ([4806374](https://github.com/timlittle/omamessenger/commit/480637400b0f9fa93680bda869a8e9a017484f46))
* **ui:** discard a stale media.fetch answer after an edit changes a message's media ([5345f55](https://github.com/timlittle/omamessenger/commit/5345f557d29ab58a4ab3d63bdc561472463ea1b1))
* **ui:** gate the list search field on window activity too, not only being edited ([73f350b](https://github.com/timlittle/omamessenger/commit/73f350bc2f4f87f01401201ad7e2f0882813c5e0))
* **ui:** keep a conversation's verified flag from being dropped in the list ([9e4c9d1](https://github.com/timlittle/omamessenger/commit/9e4c9d1c749b9362a1b40a97b2801634373c1460))
* **ui:** keep a just-scheduled message's row inside the visible chat ([e5a2390](https://github.com/timlittle/omamessenger/commit/e5a2390d8f9fb628942ab8d9e5d0e7b1e584e338))
* **ui:** keep message annotations aligned with their own row after a live insert or remove ([33f746d](https://github.com/timlittle/omamessenger/commit/33f746daee31c188af0645d7138681e51cd24b2c))
* **ui:** label the link confirmation's real host "Domain:" ([9e7bcaa](https://github.com/timlittle/omamessenger/commit/9e7bcaa14e167dc34fd7b3959731b13020b23c13))
* **ui:** let keyboard edit of a queued message change its text, not just its time ([5b64ed2](https://github.com/timlittle/omamessenger/commit/5b64ed281d765a0a9cde6a467c974679fa01435a))
* **ui:** load scheduled messages when the service is already ready ([e595387](https://github.com/timlittle/omamessenger/commit/e5953871fa34d775fe4dbb1bd8c070cb5be4c68c))
* **ui:** mark a self-chosen display name with a leading tilde ([8eb7e6f](https://github.com/timlittle/omamessenger/commit/8eb7e6fe631657e625f74a30e154042d35475bef))
* **ui:** mark an unverified contact in the new-chat list ([9c1e1d3](https://github.com/timlittle/omamessenger/commit/9c1e1d35b84a18d7571b2e4f11ce31028a3e6924))
* **ui:** mark an unverified name in the command palette ([a278f96](https://github.com/timlittle/omamessenger/commit/a278f9616c9fcda155cef8bc5daaf40bec665af3))
* **ui:** mark an unverified name in the mention picker and reply banner ([c751138](https://github.com/timlittle/omamessenger/commit/c7511389c8253fb8d3872b38c95b1099a77972fb))
* **ui:** mark an unverified name in the typing indicator ([f46ee49](https://github.com/timlittle/omamessenger/commit/f46ee4916d1ad281517ed48c3f7b2990cf7a1677))
* **ui:** mark an unverified sender in the group conversation preview ([d40e0e9](https://github.com/timlittle/omamessenger/commit/d40e0e9326ba12498e79999f116322664889c621))
* **ui:** match the "o" key's link to exactly what the bubble shows ([e3eeb72](https://github.com/timlittle/omamessenger/commit/e3eeb728be357fef829645dbb0e247a5ce160bed))
* **ui:** match the o key's links to exactly what the bubble renders, not a second raw-text pass ([ee60638](https://github.com/timlittle/omamessenger/commit/ee606382b60684f91b31812dec2914680246791e))
* **ui:** name Ctrl+L on the composer's clock button tooltip ([63ce819](https://github.com/timlittle/omamessenger/commit/63ce8199916322bb81666679fdeb1f82807029ab))
* **ui:** never let a play request settle for a download-only fetch's answer ([09f4ca8](https://github.com/timlittle/omamessenger/commit/09f4ca8c9a130a244f339f084aca3085b1fb82ea))
* **ui:** never let Qt's own clipboard reader see a foreign offer ([c231580](https://github.com/timlittle/omamessenger/commit/c2315801a78256362302afd4064944e60b3fd48d))
* **ui:** offer only sound files the helper can actually play ([e089e5e](https://github.com/timlittle/omamessenger/commit/e089e5ea2b3b20a21af693d16fd036dd7b9aee3c))
* **ui:** only guard conversation-changing actions against mistyped bursts ([e6ea31b](https://github.com/timlittle/omamessenger/commit/e6ea31b9b2980a96e671c4db8bb5171a38052fe7))
* **ui:** open file dialogs through xdg-desktop-portal, never in-process ([fbba527](https://github.com/timlittle/omamessenger/commit/fbba527e332ef415f2bfee118ccf747755932cc1))
* **ui:** open links and local files through the desktop portal, not xdg-open ([fad0ff1](https://github.com/timlittle/omamessenger/commit/fad0ff1af753fb3bbe9da463c48b3b130d1d88f8))
* **ui:** play voice notes and video through the current default audio output ([0f8dd25](https://github.com/timlittle/omamessenger/commit/0f8dd25f00568dbba2ea0e455c527ad41913a04a))
* **ui:** reclaim keyboard focus whenever the focused field is gone ([317c990](https://github.com/timlittle/omamessenger/commit/317c990532eed27bfdfb3edaf7aa6eb7110ce680))
* **ui:** recognise a native paste key without depending on Qt's own platform theme ([2c7132c](https://github.com/timlittle/omamessenger/commit/2c7132c339cbd762075279697b15c2d1e5698416))
* **ui:** record and open an autolinked url in its raw form, not escaped ([3b94250](https://github.com/timlittle/omamessenger/commit/3b9425025fd10428b707a34cd2aae4ccc06beb60))
* **ui:** refuse to paste into a read-only or rich-text field ([2239cb2](https://github.com/timlittle/omamessenger/commit/2239cb22e6d3653ae2cbc9ec89e4592da6ce9fa1))
* **ui:** remove the in-shell fallback file-picker dialogs ([ce8a7d9](https://github.com/timlittle/omamessenger/commit/ce8a7d9715a25459053f380df2b0d34c762df044))
* **ui:** remove the unverified-name tilde marker ([fefc928](https://github.com/timlittle/omamessenger/commit/fefc928bef62e42cb102af24b699f905075d4335))
* **ui:** replace quadratic trailing-character regexes with linear scans ([58ef8d6](https://github.com/timlittle/omamessenger/commit/58ef8d63635b8141e5211e64ac91a29f9cdf62d9))
* **ui:** replace two quadratic trailing-punctuation regexes with a linear scan ([8508a1a](https://github.com/timlittle/omamessenger/commit/8508a1a22e67b886b996cab5c06a0ce141837fca))
* **ui:** require a link preview's url to match an anchor's own visible text ([5643a1e](https://github.com/timlittle/omamessenger/commit/5643a1e6985c211a80fa7be16ef6bf85c5b1ec3b))
* **ui:** require Shift to archive a chat with one key ([a4bba6d](https://github.com/timlittle/omamessenger/commit/a4bba6dc0d9eb9fe5d268e9e47406be340ea7e7d))
* **ui:** route every field's native paste through the helper, not just the composer ([0342e4d](https://github.com/timlittle/omamessenger/commit/0342e4d3f46b4e0c499a763e53fc5368218e3ca1))
* **ui:** say why "Play notification sound" played nothing ([7fd615a](https://github.com/timlittle/omamessenger/commit/7fd615a7d37b827ca02ba1e319f95e374887abf3))
* **ui:** show a mismatched link's real host on its own unelided line ([e2d6283](https://github.com/timlittle/omamessenger/commit/e2d628382cafd556a7e17b0b4d8dc9befa6e3b2a))
* **ui:** snapshot the open conversation instead of storing it by reference ([872dc2c](https://github.com/timlittle/omamessenger/commit/872dc2c676fe0bfbcc225698a7c93b66f68f768c))
* **ui:** spell hidden characters as escapes, never as literal invisible characters ([9b4b629](https://github.com/timlittle/omamessenger/commit/9b4b629264b7aed1a04159ce4fcfec70eca6669b))
* **ui:** stop building a message's non-matching media views at all ([9a1149f](https://github.com/timlittle/omamessenger/commit/9a1149f2023e367b8fc19b6e04722b5adb89444e))
* **ui:** stop URL_PATTERN ending an autolink at the byte-order mark ([939a2e9](https://github.com/timlittle/omamessenger/commit/939a2e97c55a7f9d7f98b6baf351de19c59ef35c))
* **ui:** toast blocking, unblocking and reporting spam ([659bdeb](https://github.com/timlittle/omamessenger/commit/659bdeb25db89f3908226672a410ecbec09352d2))
* **ui:** warn when the composer cuts text to the length limit ([4c38129](https://github.com/timlittle/omamessenger/commit/4c38129d7a3637a3a964d5eeb0d5219d497c4aac))
* **ui:** wrap a long toast notice inside its own pill ([70689b2](https://github.com/timlittle/omamessenger/commit/70689b2aad843abd6aff8a35074ce40b1371d6b1))
* **video:** bind playability to a stream's own coded dimensions ([da328c6](https://github.com/timlittle/omamessenger/commit/da328c6d193d5f41a96bef9cd0b760edb94b47dd))
* **video:** bound the in-band sample scan's own cost to the file's size ([98da223](https://github.com/timlittle/omamessenger/commit/98da2231b3f1a4f50a963a2ba61f9ba3dd16647e))
* **video:** refuse an in-band H.264 parameter set that bypasses avcC's SPS bound ([23db803](https://github.com/timlittle/omamessenger/commit/23db803886ed543bdfee372c6810b65b0e534f63))
* **video:** require exactly one stsd sample entry, in sync with stsc ([e85995f](https://github.com/timlittle/omamessenger/commit/e85995fe30a55e1227fc7abab6a294f5dc6e9879))
* **videosafe,audiosafe:** bound every track's own sample tables, not just a video track's sample count ([3fd30b5](https://github.com/timlittle/omamessenger/commit/3fd30b50fb01712be1f04b2a76bda6a8bf344a2d))
* **videosafe,audiosafe:** check an avc1/mp4a/Opus sample entry's own children, not just its codec fourcc ([9506d24](https://github.com/timlittle/omamessenger/commit/9506d2465ab932a5f753d7c5bad78feb72bfe819))
* **videosafe:** bind avcC's own header bytes before trusting its SPS ([568d549](https://github.com/timlittle/omamessenger/commit/568d54997df9078879b35099f71a836452177a96))
* **videosafe:** bound H.264 coded size, ref frames, bit depth and chroma format ([0663cc0](https://github.com/timlittle/omamessenger/commit/0663cc039336a2052bd63afc6ea493417ae1a8be))
* **videosafe:** refuse an in-band sample shaped like avcC itself ([3352c88](https://github.com/timlittle/omamessenger/commit/3352c886b28ea9271e578e806557571cda9cf37a))
* **videosafe:** require avcC's PPS-count byte, as real encoders write it ([b5bbf32](https://github.com/timlittle/omamessenger/commit/b5bbf32c764e79342d05cf9890c97ae2dfe5d91f))
* **whatsapp:** address pin and archive app-state patches by LID, and fall back to a full resync ([38fc55e](https://github.com/timlittle/omamessenger/commit/38fc55eb039d8a1ccba07a681c4034695e160d14))
* **whatsapp:** cap a reaction's and a poll's own id before storing ([f278060](https://github.com/timlittle/omamessenger/commit/f278060cb5913dbb984f597500c7c41571c03480))
* **whatsapp:** check who sent a live edit or revoke before applying it ([7160560](https://github.com/timlittle/omamessenger/commit/7160560369318d9c3b6bbd0f456161bf4d8d8758))
* **whatsapp:** drop a message whose id is reused by a different sender instead of delivering it ([d817a86](https://github.com/timlittle/omamessenger/commit/d817a86d406b22acba2c54d154c5eec264841185))
* **whatsapp:** drop the unused receiver name golangci-lint flagged ([3b65098](https://github.com/timlittle/omamessenger/commit/3b650983047c92e68b6cc16505b9452b3d82f72c))
* **whatsapp:** keep a WhatsApp chat from showing pinned and archived at once ([ade808a](https://github.com/timlittle/omamessenger/commit/ade808a7f4417466ab8666f255261a71ae3ae205))
* **whatsapp:** keep pin, archive and delete local when WhatsApp's chat state can't be verified ([53f98de](https://github.com/timlittle/omamessenger/commit/53f98deaa9f299e53dfd5cef55882a70edc42fd9))
* **whatsapp:** keep the composer's attachment hint after a reconnect ([e23a717](https://github.com/timlittle/omamessenger/commit/e23a71793c3d9649d8d1cb2de2de8c85f225d27b))
* **whatsapp:** keep the first sender on a reused message id ([52cd665](https://github.com/timlittle/omamessenger/commit/52cd6654da73e2b4f5b812cefe15c79c7d560c75))
* **whatsapp:** retry app-state patches after a catch-up and classify failures ([57a6d36](https://github.com/timlittle/omamessenger/commit/57a6d3608126468e71e78a634286fde8226915a4))
* **whatsapp:** show a business name as given, regardless of verification level ([67770ad](https://github.com/timlittle/omamessenger/commit/67770ad0c1cadc214feb19e224e6bae4ef44fef0))
* **whatsapp:** stop treating any business name as verified ([6456284](https://github.com/timlittle/omamessenger/commit/64562845967b27c7c686df9394aa02c11c6c412f))
* **whatsapp:** three small gaps a security review found ([532b13c](https://github.com/timlittle/omamessenger/commit/532b13c0f33adf054cc7548c92e333b58c127a40))

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
