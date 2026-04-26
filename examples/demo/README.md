# Demo Story — 霧山懸案

An original short story created specifically to demonstrate Novel Assistant's core features.
All content is original and copyright-free.

## Files

```
demo/
├── characters/林逸.md       # Character profile
├── worldbuilding/霧山懸案.md # World setting
├── style/主線敘事.md         # Writing style guide
└── chapters/第一章_抵達.md   # Demo chapter
```

## How to use

Copy the contents of this folder into your `data/` directory, then click **重新索引** (Reindex).

```bash
cp -r examples/demo/characters  data/characters
cp -r examples/demo/worldbuilding data/worldbuilding
cp -r examples/demo/style        data/style
cp -r examples/demo/chapters     data/chapters
```

Then open the **審查** (Review) or **評分** (Story Health) page and use `第一章_抵達.md` as the input.

## What this demo is designed to show

The chapter contains three deliberate issues so the tool has concrete things to find:

| Issue | What it is | Which feature catches it |
|---|---|---|
| **Character inconsistency** | 林逸 is defined as cautious and never reveals his identity to strangers — but he tells a stranger his name, mission, and that the jade is key evidence, unprompted | Character behavior review, Story Health |
| **Unresolved hook** | A jade pendant with unknown runes that he has investigated for three years — origin and meaning never explained in this chapter | Hook Tracker |
| **Pacing problem** | Five consecutive lines of static scenery description (sunset light, dust, tree, wind chime, puddle) with no action or dialogue — the story stalls | Story Health pacing score |

A well-tuned run should return a Story Health score somewhere in the 55–72 range with High or Medium confidence, and Hook Tracker should surface the jade pendant as a candidate.

## Story synopsis

林逸 (Lin Yi) is an imperial investigator sent undercover to a remote mountain town to look into a series of disappearances. On his first evening, a mysterious stranger in a tavern recognizes an unusual jade pendant he carries — and Lin Yi inexplicably reveals everything about himself and his mission, breaking every rule he lives by.
