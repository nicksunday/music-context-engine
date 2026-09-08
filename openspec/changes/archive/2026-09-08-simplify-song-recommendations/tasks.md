## 1. Simplify recommendation presentation

- [x] 1.1 Make hidden sections respect their hidden state so choice cards appear only on the entry page; preserve shared navigation.
- [x] 1.2 Render song batches as flat single-column rows with separators and responsive wrapping; preserve album card layout.
- [x] 1.3 Remove playlist naming, authorization, creation, and retry UI plus unused browser MusicKit initialization and handlers; preserve direct song links and feedback.

## 2. Validate and deliver

- [x] 2.1 Run relevant web tests and JavaScript syntax checks; verify empty, current, and retained song batches still render and feedback works.
- [x] 2.2 Check entry, song, and album routes at desktop and narrow widths for chooser visibility, single-column songs, readable long titles, no playlist controls, and unchanged album cards.
- [x] 2.3 Rebuild embedded assets and restart the local web server using the project's lifecycle instructions; verify the served page includes the changes.
