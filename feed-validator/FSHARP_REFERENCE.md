# F# クイックリファレンス

feed-validator 実装で使う F# 構文のリファレンス。

---

## 基本構文

### 変数束縛

```fsharp
// 不変（推奨）
let x = 10
let name = "F#"

// 可変（必要な場合のみ）
let mutable counter = 0
counter <- counter + 1
```

### 関数定義

```fsharp
// 基本形
let add x y = x + y

// 型注釈付き
let add (x: int) (y: int) : int = x + y

// 複数行
let complexFunction x =
    let intermediate = x * 2
    let result = intermediate + 1
    result  // 最後の式が戻り値

// ラムダ式
let double = fun x -> x * 2
```

### 型推論

```fsharp
// F# は型を推論する
let numbers = [1; 2; 3]        // int list
let greet name = "Hello, " + name  // string -> string
```

---

## コレクション

### リスト（不変・連結リスト）

```fsharp
// 作成
let numbers = [1; 2; 3; 4; 5]
let empty = []
let range = [1..10]
let generated = [for i in 1..5 -> i * i]

// 操作
let first = List.head numbers      // 1
let rest = List.tail numbers       // [2; 3; 4; 5]
let prepended = 0 :: numbers       // [0; 1; 2; 3; 4; 5]
let concatenated = [1; 2] @ [3; 4] // [1; 2; 3; 4]

// 高階関数
numbers |> List.map (fun x -> x * 2)      // [2; 4; 6; 8; 10]
numbers |> List.filter (fun x -> x > 2)   // [3; 4; 5]
numbers |> List.fold (+) 0                // 15
numbers |> List.find (fun x -> x > 3)     // 4
numbers |> List.tryFind (fun x -> x > 10) // None
```

### 配列（可変・インデックスアクセス）

```fsharp
let arr = [| 1; 2; 3 |]
let first = arr.[0]
arr.[0] <- 10  // 変更可能

// 高階関数は List と同じ
arr |> Array.map (fun x -> x * 2)
```

### Seq（遅延評価）

```fsharp
// 大量データや無限シーケンスに
let infinite = Seq.initInfinite (fun i -> i * 2)
let first10 = infinite |> Seq.take 10 |> Seq.toList
```

---

## Option 型

### 基本

```fsharp
type Option<'T> =
    | Some of 'T
    | None

// 使用例
let maybeName: string option = Some "F#"
let noName: string option = None
```

### パターンマッチ

```fsharp
let greet maybeNam =
    match maybeName with
    | Some name -> sprintf "Hello, %s!" name
    | None -> "Hello, stranger!"
```

### 便利な関数

```fsharp
// デフォルト値
let name = Option.defaultValue "unknown" maybeName

// map: Some の中身を変換
let upperName = maybeName |> Option.map (fun s -> s.ToUpper())

// bind: Option を返す関数を連結
let tryParse (s: string) : int option =
    match System.Int32.TryParse(s) with
    | true, n -> Some n
    | false, _ -> None

"42" |> Some |> Option.bind tryParse  // Some 42

// null からの変換
let fromNull = Option.ofObj nullableValue
```

---

## Result 型

### 基本

```fsharp
type Result<'T, 'Error> =
    | Ok of 'T
    | Error of 'Error

// 使用例
let success: Result<int, string> = Ok 42
let failure: Result<int, string> = Error "Something went wrong"
```

### パターンマッチ

```fsharp
let handleResult result =
    match result with
    | Ok value -> sprintf "Success: %d" value
    | Error msg -> sprintf "Failed: %s" msg
```

### Railway Oriented Programming

```fsharp
// bind: 成功時のみ次の関数を実行
let validatePositive x =
    if x > 0 then Ok x
    else Error "Must be positive"

let validateEven x =
    if x % 2 = 0 then Ok x
    else Error "Must be even"

let validateNumber x =
    x
    |> validatePositive
    |> Result.bind validateEven

// map: 成功時の値を変換
Ok 10 |> Result.map (fun x -> x * 2)  // Ok 20
Error "fail" |> Result.map (fun x -> x * 2)  // Error "fail"

// mapError: エラーを変換
Error "fail" |> Result.mapError (fun s -> s.ToUpper())  // Error "FAIL"
```

---

## 判別共用体（Discriminated Union）

### 定義

```fsharp
// 単純な列挙
type Color = Red | Green | Blue

// 値を持つケース
type Shape =
    | Circle of radius: float
    | Rectangle of width: float * height: float
    | Point

// 再帰的な型
type Tree<'T> =
    | Leaf of 'T
    | Node of Tree<'T> * Tree<'T>
```

### パターンマッチ

```fsharp
let area shape =
    match shape with
    | Circle r -> System.Math.PI * r * r
    | Rectangle (w, h) -> w * h
    | Point -> 0.0

// 網羅性チェック: 全ケースをカバーしないと警告
```

### 単一ケース共用体（ラッパー型）

```fsharp
// 型安全な ID
type UserId = UserId of int
type OrderId = OrderId of int

let userId = UserId 123
let orderId = OrderId 123

// userId = orderId  // コンパイルエラー！型が違う

// 中身を取り出す
let (UserId id) = userId
```

---

## レコード型

### 定義

```fsharp
type Person = {
    Name: string
    Age: int
    Email: string option
}
```

### 作成

```fsharp
let person = {
    Name = "Alice"
    Age = 30
    Email = Some "alice@example.com"
}
```

### 更新（コピー＆変更）

```fsharp
// 元のレコードは変更されない
let olderPerson = { person with Age = 31 }
```

### 分解

```fsharp
let { Name = n; Age = a } = person
printfn "%s is %d years old" n a
```

---

## パイプライン演算子

### |> （前方パイプ）

```fsharp
// 左の値を右の関数に渡す
// x |> f  は  f x  と同じ

let result =
    "hello"
    |> String.toUpper
    |> String.length
    |> fun len -> len * 2

// ネストよりも読みやすい
// let result = (String.length (String.toUpper "hello")) * 2
```

### ||> （2引数パイプ）

```fsharp
// タプルを2引数関数に渡す
(1, 2) ||> (+)  // 3
```

### >> （関数合成）

```fsharp
// 関数を合成して新しい関数を作る
let toUpperLength = String.toUpper >> String.length

"hello" |> toUpperLength  // 5
```

---

## パターンマッチ

### match 式

```fsharp
let describe x =
    match x with
    | 0 -> "zero"
    | 1 -> "one"
    | n when n < 0 -> "negative"
    | _ -> "many"  // ワイルドカード
```

### リストパターン

```fsharp
let describeList lst =
    match lst with
    | [] -> "empty"
    | [x] -> sprintf "single: %d" x
    | [x; y] -> sprintf "pair: %d, %d" x y
    | x :: rest -> sprintf "head: %d, tail has %d items" x (List.length rest)
```

### タプルパターン

```fsharp
let describe point =
    match point with
    | (0, 0) -> "origin"
    | (x, 0) -> sprintf "on x-axis at %d" x
    | (0, y) -> sprintf "on y-axis at %d" y
    | (x, y) -> sprintf "at (%d, %d)" x y
```

### アクティブパターン

```fsharp
// カスタムパターンを定義
let (|Even|Odd|) n =
    if n % 2 = 0 then Even else Odd

let describe n =
    match n with
    | Even -> "even"
    | Odd -> "odd"
```

---

## 非同期処理

### async 式

```fsharp
let fetchUrl (url: string) =
    async {
        use client = new System.Net.Http.HttpClient()
        let! response = client.GetStringAsync(url) |> Async.AwaitTask
        return response.Length
    }

// 実行
let length = fetchUrl "https://example.com" |> Async.RunSynchronously
```

### task 式（.NET 互換）

```fsharp
open System.Threading.Tasks

let fetchUrlTask (url: string) =
    task {
        use client = new System.Net.Http.HttpClient()
        let! response = client.GetStringAsync(url)
        return response.Length
    }
```

### F# 10: and! による並行実行

```fsharp
let fetchBoth url1 url2 =
    task {
        let! result1 = fetchUrlTask url1
        and! result2 = fetchUrlTask url2  // 並行実行！
        return result1 + result2
    }
```

---

## モジュール

### 定義

```fsharp
module Validation =

    let isPositive x = x > 0

    let isEven x = x % 2 = 0

    let isPositiveEven x =
        isPositive x && isEven x
```

### 使用

```fsharp
// 完全修飾
Validation.isPositive 5

// open して使用
open Validation
isPositive 5

// モジュール別名
module V = Validation
V.isPositive 5
```

### 名前空間

```fsharp
namespace FeedValidator.Domain

type FeedFormat = RSS2 | Atom | JSONFeed

module Feed =
    let parse content = ...
```

---

## エラー処理のパターン

### Option で欠損値を扱う

```fsharp
let tryFindUser id =
    users
    |> List.tryFind (fun u -> u.Id = id)
    |> Option.map (fun u -> u.Name)
    |> Option.defaultValue "Unknown"
```

### Result でエラーを伝播

```fsharp
let validateAndProcess input =
    input
    |> validate
    |> Result.bind transform
    |> Result.bind save

// 複数エラーを収集
let validateAll input =
    let errors =
        [
            if String.IsNullOrEmpty input.Name then
                yield "Name is required"
            if input.Age < 0 then
                yield "Age must be positive"
        ]
    if List.isEmpty errors then
        Ok input
    else
        Error errors
```

### try-with（例外をキャッチ）

```fsharp
let safeParseInt (s: string) =
    try
        Ok (int s)
    with
    | :? System.FormatException as ex ->
        Error (sprintf "Invalid format: %s" ex.Message)
    | ex ->
        Error (sprintf "Unexpected error: %s" ex.Message)
```

---

## デバッグ Tips

### printfn

```fsharp
printfn "Value: %d" 42
printfn "String: %s" "hello"
printfn "Any: %A" [1; 2; 3]  // %A は何でも表示
```

### |> を使ったデバッグ

```fsharp
let result =
    input
    |> step1
    |> fun x -> printfn "After step1: %A" x; x  // 値を表示して通す
    |> step2
    |> step3
```

### F# Interactive (FSI)

```bash
dotnet fsi

> let x = 1 + 2;;
val x: int = 3

> [1..10] |> List.map (fun x -> x * 2);;
val it: int list = [2; 4; 6; 8; 10; 12; 14; 16; 18; 20]
```

---

## よく使うライブラリ関数

### String

```fsharp
String.IsNullOrEmpty s
String.IsNullOrWhiteSpace s
s.ToUpper()
s.ToLower()
s.Trim()
s.Split(',')
s.StartsWith("prefix")
s.Contains("substring")
sprintf "Formatted: %s %d" name age
```

### List / Array / Seq 共通

| 関数 | 説明 |
|------|------|
| `map` | 各要素を変換 |
| `filter` | 条件に合う要素だけ抽出 |
| `fold` | 累積計算 |
| `reduce` | fold の初期値なし版 |
| `find` | 条件に合う最初の要素（なければ例外） |
| `tryFind` | 条件に合う最初の要素（Option） |
| `exists` | 条件に合う要素があるか |
| `forall` | 全要素が条件を満たすか |
| `head` | 最初の要素 |
| `tail` | 最初以外 |
| `length` | 長さ |
| `isEmpty` | 空かどうか |
| `collect` | map + flatten |
| `choose` | map + filter (None を除去) |
| `mapi` | インデックス付き map |
| `iter` | 副作用のある処理 |
| `sortBy` | ソート |
| `groupBy` | グループ化 |
| `distinct` | 重複除去 |

---

## 参考リンク

- [F# Cheat Sheet](https://dungpa.github.io/fsharp-cheatsheet/)
- [F# for Fun and Profit](https://fsharpforfunandprofit.com/)
- [F# Core Library](https://fsharp.github.io/fsharp-core-docs/)
