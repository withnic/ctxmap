## コード探索ルール（ctxmap によるトークン節約）

<!-- Claude Code のスキル（skills/ctxmap、`make install-skill`）を入れている場合は、このテンプレートの代わりに「コードを探索するときは、ファイルを Read する前に ctxmap スキルを使うこと」の1行だけで足ります。 -->

このリポジトリでは、コードを読む前に `ctxmap` で骨格と依存関係を確認すること。ファイル全体の Read は最後の手段とする。

### 基本フロー
1. **全体把握**: 最初に `ctxmap map [dir]` を1回だけ実行し、型定義・関数シグネチャ・Docコメント要約を把握する。大きなリポジトリでは先に `ctxmap map --summary` でディレクトリ構成をつかみ、対象ディレクトリに絞って `map` する（例: `ctxmap map src/api`）。
   - ファイル見出しの直下の `> ` で始まる行はそのファイルの役割、`--summary` の各ディレクトリの下の `> ` 行はパッケージの役割。これを見て、読むファイルを選ぶ。
2. **影響範囲の確認**: 変更対象を決めたら `ctxmap deps --target <file|dir|package>` で「依存先」と「被参照元」を確認する。間接的な影響まで見る場合は `--depth 2`（`0` で無制限）を付ける。依存関係を知るために import 文を Read/Grep で追わない。
3. **実装の確認**: 中身が必要な関数・型だけを `ctxmap slice <Symbol> [<Symbol>...]` で取得する。複数のシンボルは1回でまとめて取得する。メソッドは `Recv.Method` / `Class.method` 形式で指定できる。
4. **Read は限定的に**: `map` の各宣言には `#L開始-終了` が付いている。Read する場合は `offset`/`limit` をその範囲に絞る。ファイル全体を Read するのは、編集直前のファイルと、ctxmap が対応していない言語・ファイルだけにする。

### レビュー・作業途中の確認
- 未コミットの変更だけを見る: `ctxmap map --changed`
- ブランチ（PR）全体の変更を見る: `ctxmap map --changed --base main`
- 変更ファイルの依存関係: `ctxmap deps --changed --base main`

### コマンド早見表
| 目的 | コマンド |
|---|---|
| ディレクトリごとの概要 | `ctxmap map --summary` |
| 出力の上限を変える / なくす | `--max-lines N` / `--max-lines 0`（`map` と `deps`、既定は500行） |
| 行番号を出さない / .gitignore を無視する | `--no-lines` / `--no-gitignore` |
| シンボル名がパスと紛らわしいとき | `ctxmap slice <Symbol> --path <dir>` |
| 骨格（シグネチャ・型・Doc要約・行番号） | `ctxmap map [path]` |
| 公開APIだけ（Go） | `ctxmap map [path] --exported` |
| Docコメント全文 / なし | `--full-doc` / `--no-doc` |
| ローカル依存グラフ | `ctxmap deps [path]` |
| 特定対象の依存先・被参照元 | `ctxmap deps --target <file|dir|package> [--depth N]` |
| 定義の本体を取得（複数可） | `ctxmap slice <Name|Recv.Method|Class.member>... [path]` |
| テストも対象にする | `--include-tests`（全コマンド共通） |
| その場で除外する | `--exclude '<pattern>'`（全コマンド共通、複数指定可） |

### 守ること
- 同じセッションで同じディレクトリの `ctxmap map` を繰り返さない（前回の出力を再利用する）。コードを編集した後は `--changed` で変更分だけを再確認する。
- 出力の末尾に `... (truncated at --max-lines ...)` が出たら、続きを `--max-lines 0` で取得するのではなく、`--summary` やサブディレクトリの指定で対象を絞る。
- シンボル名が不確かな場合は先に `map` で確認する。`slice` で見つからない場合は stderr に類似候補が出るので、それを使う。
- TS/JS の `map` には export された宣言だけが表示される。非公開の関数を探す場合は `slice <name>` を使う。
- 対応言語: Go（`.go`）、TypeScript/JavaScript（`.ts` `.tsx` `.js` `.jsx`）。`node_modules` `vendor` `dist` `build` `.next` `.git` と、`.gitignore` および `.ctxmapignore` の対象は自動で除外される。生成コードなど、エージェントに見せる必要のないものは `.ctxmapignore` に追記する。
