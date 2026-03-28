#### procinfo version
awk 'BEGIN { print PROCINFO["version"] }'

#### argind across files
printf 'a\n' > one.txt
printf 'b\n' > two.txt
awk '{ print ARGIND ":" FILENAME ":" $0 }' one.txt two.txt

#### ignorecase pattern matching
printf 'Foo\nbar\n' | awk 'BEGIN { IGNORECASE = 1 } /foo/ { print $0 }'

#### fieldwidths splitting
printf 'abc123xyz\n' | awk 'BEGIN { FIELDWIDTHS = "3 3" } { print $1 "-" $2 "-" $3 }'

#### fs assignment restores splitting after fieldwidths
printf 'ab,cd\n' | awk 'BEGIN { FIELDWIDTHS = "2 2"; FS = "," } { print NF ":" $1 ":" $2 }'

#### fpat splitting
printf 'a=1 b=22\n' | awk 'BEGIN { FPAT = "[[:alpha:]]+|[0-9]+" } { print NF ":" $1 ":" $2 ":" $3 ":" $4 }'

#### fs assignment restores splitting after fpat
printf 'ab,cd\n' | awk 'BEGIN { FPAT = "[[:alpha:]]"; FS = "," } { print NF ":" $1 ":" $2 }'
