# Hoshobako （保証書箱） — Warranty & Expiry Ledger

**Every warranty card you own, in one place — on your own computer, nowhere else.**

Hoshobako is a local-first ledger for the dates that quietly cost you money
when you miss them: product warranties, passport and licence expiries,
food and medicine use-by dates, and contract cancel-by deadlines.

Most warranty apps ask you to create an account and upload your receipts to
someone's cloud. If that service shuts down — or locks you out — your records
are gone. Hoshobako is the opposite:

- **One binary.** No install, no runtime, no database server.
- **No account, no cloud, no network.** The program contains no network
  client code at all. The web UI is served on `127.0.0.1` only.
- **Your data is one JSON file** in your own data folder, written
  atomically so a crash can never truncate it.
- **Open exports.** JSON (full backup, restorable), CSV, and ICS —
  import the ICS into the calendar you already use and *it* reminds you
  30 and 7 days before each expiry. An offline tool should not need to
  run a server to remind you.
- **Receipt photos** are copied into your own data folder, never uploaded.

## Built to outlive its maker

Hoshobako has no server to shut down, no account to lock, and no company
that has to survive for your records to survive. If this project
disappeared tomorrow, the binary on your disk keeps working, your ledger
stays readable as plain JSON, and the source is MIT-licensed so anyone can
rebuild or fork it. Software you keep should not depend on a service
someone else can switch off.

## Quick start

Download the binary for your platform from the
[Releases](https://github.com/shizukutanaka/hoshobako/releases) page and run it:

```
./hoshobako
```

It opens the ledger in your browser at `http://127.0.0.1:8771`
(if that port is taken it picks the next free one, and if Hoshobako is
already running it just opens the existing window).

Add an item with its purchase date and warranty length in months, or with
an explicit expiry date — the explicit date wins when both are set.

## Command line

The same binary works as a CLI:

```
hoshobako add -name "Air conditioner" -purchased 2025-04-01 -months 60 -store "Denkiya"
hoshobako list          # everything, soonest expiry first
hoshobako due           # expired, or expiring within 30 days
hoshobako export ledger.csv   # CSV is the default export format
hoshobako export              # CSV to stdout
hoshobako export ics reminders.ics
hoshobako export json backup.json
hoshobako import backup.json   # restore (a non-empty ledger is backed up first)
hoshobako version
```

## Data location

| Platform | Folder |
|---|---|
| Windows | `%APPDATA%\Hoshobako\` |
| macOS | `~/Library/Application Support/Hoshobako/` |
| Linux | `$XDG_DATA_HOME/hoshobako/` or `~/.local/share/hoshobako/` |

Set `HOSHOBAKO_DATA_DIR` to override. The folder holds `hoshobako.json`
and a `photos/` subfolder. Copy the folder and you have copied your ledger.

## Build from source

Requires Go 1.22 or later. No third-party dependencies.

```
go build -o hoshobako .
go test ./...
```

## 日本語

保証書・パスポート・免許・食品や薬の期限・契約の解約期限を、自分の
パソコンの中だけで管理する台帳です。アカウントもクラウドもなく、
データは自分のフォルダにある JSON ファイル1つだけ。書き出しは
JSON / CSV / ICS に対応し、ICS を普段のカレンダーに入れれば、
期限の30日前と7日前にカレンダーが知らせてくれます。

## License

MIT — see [LICENSE](LICENSE).
