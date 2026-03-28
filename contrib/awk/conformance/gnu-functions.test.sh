#### gensub global replacement
awk 'BEGIN { print gensub("([0-9]+)", "<\\1>", "g", "item42 batch7") }'

#### gensub defaults target to current record
printf 'item42\n' | awk '{ print gensub("([0-9]+)", "<\\1>", "g") }'

#### strftime and mktime
TZ=UTC awk 'BEGIN { print strftime("%Y-%m-%d %H:%M:%S", 0, 1); print mktime("1970 01 02 00 00 00") }'

#### mktime honors DST datespec component
TZ=America/New_York awk 'BEGIN { print mktime("2024 07 01 12 00 00 0") - mktime("2024 07 01 12 00 00 1") }'
