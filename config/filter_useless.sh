tr -cs '[:alnum:]' '\n' < input.txt | sort | uniq > temp && mv temp input.txt
