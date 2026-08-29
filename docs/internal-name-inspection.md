# Internal archive name inspection

Design note for the next naming pass. Scan archive metadata only; never extract image bodies merely to classify a title.

Priority:
1. nested ZIP/RAR filenames
2. top-level directory names
3. meaningful image filenames
4. outer archive filename fallback

Japanese-containing internal candidates receive a preference bonus. Repeated normalized series candidates across multiple entries receive consensus confidence. Numeric-only image names and generic folders such as images/pages/scans are ignored.

Limits keep scan work bounded: metadata only, at most 5000 entries inspected and 256 naming candidates retained per archive.
