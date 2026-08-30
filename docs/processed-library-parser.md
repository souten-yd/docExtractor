# Processed-library filename parsing

`Existing-file reprocess` and `multi-library management` operate on archives that were already unpacked/repacked by a previous organizer.

Rules:

- Never inspect archive members for series identity.
- Use the filesystem archive filename as the primary naming evidence.
- Keep parsing behavior compatible with docExtractor v0.2.11 for ambiguous legacy suffixes.
- Only simple edition/side-material suffixes are removed from series identity: color/semicolor/monochrome, alternate/rescan, alternate cooking, fix/revised, side story/extra story, bonus/extra, single-page/spread variants.
- Raw archive processing uses the normal classifier and may evolve independently.

This separation prevents changes made for nested/raw archive normalization from silently reorganizing an already-processed library.
