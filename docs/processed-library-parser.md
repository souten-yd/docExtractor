# Processed-library filename parsing

`Existing-file reprocess` and `multi-library management` operate on archives that were already unpacked/repacked by a previous organizer.

Rules:

- Never inspect archive members for series identity.
- Use the filesystem archive filename as the primary naming evidence.
- Recognize volume-letter suffixes (`v06s`) and trailing volume ranges (`第01-02巻s`). A range is not a single volume and cannot participate in single-volume replacement.
- Simple edition/side-material suffixes are removed from series identity: color/semicolor/monochrome, alternate/rescan, alternate cooking, fix/revised, side story/extra story, bonus/extra, single-page/spread variants.
- Raw archive processing uses the normal classifier and may evolve independently.

This separation prevents changes made for nested/raw archive normalization from silently reorganizing an already-processed library.


Folder clustering also receives filename author evidence. Boundary star ratings are preserved independently of title identity; see [series-folder-repair.md](series-folder-repair.md). Numbered sequel titles retain distinct same-volume identities even when grouped in a family folder.
