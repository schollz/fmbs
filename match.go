package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"fmt"
	"github.com/arbovm/levenshtein"
	"github.com/cheggaaa/pb"
	_ "github.com/mattn/go-sqlite3"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

//GLOBALS
var findings_matches []string
var findings_leven []int
var wg sync.WaitGroup
var tuple_length int
var file_tuple_length int
var dbPath string

func abs(x int) int {
	if x < 0 {
		return -x
	} else if x == 0 {
		return 0 // return correctly abs(-0)
	}
	return x
}

func lineCounter(r io.Reader) (int, error) {
	buf := make([]byte, 8196)
	count := 0
	lineSep := []byte{'\n'}

	for {
		c, err := r.Read(buf)
		if err != nil && err != io.EOF {
			return count, err
		}

		count += bytes.Count(buf[:c], lineSep)

		if err == io.EOF {
			break
		}
	}

	return count, nil
}

func generateHash(path string) {
	inFile2, _ := os.Open(path)
	numLines, _ := lineCounter(inFile2)
	inFile2.Close()

	inFile, _ := os.Open(path)
	defer inFile.Close()
	scanner := bufio.NewScanner(inFile)
	scanner.Split(bufio.ScanLines)
	mm := make(map[string]string)

	fmt.Printf("Building map...\n")
	bar := pb.StartNew(numLines)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		bar.Increment()
		s := strings.Replace(scanner.Text(), "/", "", -1)
		//addToCache("keys.list", s)
		partials := getPartials(s)
		for i := 0; i < len(partials); i++ {
			_, ok := mm[partials[i]]
			if ok == true {
				mm[partials[i]] = mm[partials[i]] + " " + strconv.Itoa(lineNum)
			} else {
				mm[partials[i]] = strconv.Itoa(lineNum)

			}
			//addToCache(partials[i], strconv.Itoa(lineNum))
		}
	}
	bar.FinishPrint("Finished.\n")
	fmt.Printf("Building cache...")
	for k := range mm {
		//fmt.Printf("%v : %v\n", k, mm[k])
		addToCache(k[0:file_tuple_length], k+" "+mm[k])
	}
}

func generateHash2(path string) {

	inFile, _ := os.Open(path)
	defer inFile.Close()
	scanner := bufio.NewScanner(inFile)
	scanner.Split(bufio.ScanLines)

	words := make(map[string]int)
	tuples := make(map[string]int)
	numTuples := 0
	numWords := 0

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		s := strings.Replace(scanner.Text(), "/", "", -1)
		s = strings.Replace(s, "'","", -1)

		_, ok := words[s]
		if ok == false {
			words[s] = numWords
			numWords++
		}

		partials := getPartials(s)
		for i := 0; i < len(partials); i++ {
			_, ok := tuples[partials[i]]
			if ok == false {
				tuples[partials[i]] = numTuples
				numTuples++
			}
		}
	}

	cmd_start := `.echo OFF
PRAGMA cache_size = 800000;
PRAGMA synchronous = OFF;
PRAGMA journal_mode = OFF;
PRAGMA locking_mode = EXCLUSIVE;
PRAGMA count_changes = OFF;
PRAGMA temp_store = MEMORY;
PRAGMA auto_vacuum = NONE;

BEGIN;
CREATE TABLE 'tuples' ('id' INTEGER PRIMARY KEY, 'tuple' VARCHAR(7) NOT NULL);
CREATE TABLE 'words' ('id' INTEGER PRIMARY KEY,'word' VARCHAR(100) NOT NULL);
CREATE TABLE 'words_tuples' ('id' INTEGER PRIMARY KEY AUTOINCREMENT,'word_id' INTEGER,'tuple_id' INTEGER);
COMMIT;

BEGIN;`

	cmd_end := `COMMIT;

create index idx1 on tuples(tuple);
create index idx2 on words_tuples(word_id,tuple_id);
.exit
`
	fmt.Println(cmd_start)
	for k, v := range words {
		fmt.Printf("INSERT INTO words (id,word) values (%v,'%v');\n", v, k)
		partials := getPartials(k)
		for i := 0; i < len(partials); i++ {
			fmt.Printf("INSERT INTO words_tuples (word_id,tuple_id) values (%v,%v);\n", v, tuples[partials[i]])
		}
	}

	for k, v := range tuples {
		fmt.Printf("INSERT INTO tuples (id,tuple) values (%v,'%v');\n", v, k)
	}
	fmt.Println(cmd_end)

}

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}

func addToCache(spartial string, s string) {
	f, err := os.OpenFile("cache/"+spartial, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		panic(err)
	}

	defer f.Close()

	if _, err = f.WriteString(s + "\n"); err != nil {
		panic(err)
	}
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
			break
		}
	}
	return false
}

func getPartials(s string) []string {
	partials := make([]string, 100000)
	num := 0
	s = strings.ToLower(s)
	s = strings.Replace(s, "/", "", -1)
	s = strings.Replace(s, " ", "", -1)
	s = strings.Replace(s, "the", "", -1)
	s = strings.Replace(s, "by", "", -1)
	s = strings.Replace(s, "dr", "", -1)
	s = strings.Replace(s, "of", "", -1)
	slen := len(s)
	if slen <= tuple_length {
		if slen <= 3 {
			partials[num] = "zzz"
			num = num + 1
		} else {
			for i := 0; i <= slen-3; i++ {
				partials[num] = s[i : i+3]
				num = num + 1
			}
		}
	} else {
		for i := 0; i <= slen-tuple_length; i++ {
			partials[num] = s[i : i+tuple_length]
			num = num + 1
		}
	}
	return partials[0:num]
}

func removeDuplicates(a []int) []int {
	result := []int{}
	seen := map[int]int{}
	for _, val := range a {
		if _, ok := seen[val]; !ok {
			result = append(result, val)
			seen[val] = val
		}
	}
	return result
}

func ReadLine(file string, lineNum int) (line string, lastLine int, err error) {
	r, _ := os.Open(file)
	defer r.Close()
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		lastLine++
		if lastLine == lineNum {
			// you can return sc.Bytes() if you need output in []bytes
			return sc.Text(), lastLine, sc.Err()
		}
	}
	return line, lastLine, io.EOF
}

func ReadLines(file string, lineNums []int) []string {
	matches := make([]string, 100000)
	num := 0
	lastLine := 0
	r, _ := os.Open(file)
	defer r.Close()
	sc := bufio.NewScanner(r)
outerLoop:
	for sc.Scan() {
		lastLine++
	innerLoop:
		for i := 0; i < len(lineNums); i++ {
			if lastLine == lineNums[i] {
				matches[num] = sc.Text()
				num++
				lineNums = lineNums[:i+copy(lineNums[i:], lineNums[i+1:])]
				//fmt.Printf("lineNums:%v %v\n", lineNums, len(lineNums))
				break innerLoop
			}
		}
		if len(lineNums) < 1 {
			break outerLoop
		}
	}
	//fmt.Printf("lastLine:%v %v\n", lastLine, matches[0:num])
	return matches[0:num]
}

func getIndiciesFromPartial(partials []string, path string) []int {
	indexMatches := make([]int, 100000)
	numm := 0
	for i := 0; i < len(partials); i++ {

		inFile, _ := os.Open(path + partials[i][0:file_tuple_length])
		defer inFile.Close()
		scanner := bufio.NewScanner(inFile)
		scanner.Split(bufio.ScanLines)

		for scanner.Scan() {
			scan := scanner.Text()
			if partials[i] == scan[0:tuple_length] {
				for _, k := range strings.Split(scan[tuple_length:], " ") {
					indexMatches[numm], _ = strconv.Atoi(k)
					numm++
				}
			}
		}

	}
	//fmt.Printf("\nIndex matches: %v\n", indexMatches[0:numm])
	indexMatches = removeDuplicates(indexMatches[0:numm])
	//fmt.Printf("\nIndex matches: %v\n", indexMatches)
	return indexMatches

}

func getMatch2(s string, path string) (string, int) {
	start := time.Now()
	partials := getPartials(s)

	fmt.Printf("\nPartials took %s %v\n", time.Since(start), path)
	//fmt.Printf("Partials: %v", partials)
	runtime.GOMAXPROCS(8)
	N := 8

	//start = time.Now()
	matches := make([]string, 100000)
	numm := 0

	//start = time.Now()
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		panic(err)
	}
	defer db.Close()


	//orStatment := "tuples.tuple like '" + strings.Join(partials, "' or tuples.tuple like '") + "'"
	//cmd := "SELECT distinct words.word FROM  words_tuples LEFT JOIN words ON words_tuples.word_id = words.id LEFT JOIN tuples ON words_tuples.tuple_id = tuples.id WHERE " + orStatment
	start = time.Now()
	orStatement := "tuple = '" + strings.Join(partials, "' or tuple = '") + "'"
	cmd := "select id from tuples indexed by idx1 WHERE " + orStatement
	cmd = "select id from tuples indexed by idx1 WHERE tuple in ('" + strings.Join(partials, "','") + "')"

	//fmt.Println(cmd)
	rows, err := db.Query(cmd)
	indexes := make([]string,10000)
	num2 := 0
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var word string
		if err := rows.Scan(&word); err != nil {
			panic(err)
		}
		//fmt.Printf("%s\n", word)
		indexes[num2] = word
		num2++
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	indexes = indexes[0:num2]
	fmt.Printf("\nDatabase search 1 took %s \n", time.Since(start))
	//fmt.Printf("\nindexes: %v\n",indexes)

	start = time.Now()
	cmd = "SELECT DISTINCT word_id FROM words_tuples INDEXED BY idx2 WHERE tuple_id IN (" + strings.Join(indexes, ",") + ")"
	//fmt.Println(cmd)
	rows, err = db.Query(cmd)
	indexes2 := make([]string,10000)
	num3 := 0
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var word string
		if err := rows.Scan(&word); err != nil {
			panic(err)
		}
		//fmt.Printf("%s\n", word)
		indexes2[num3] = word
		num3++
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	indexes2 = indexes2[0:num3]
	fmt.Printf("\nDatabase search 2 took %s \n", time.Since(start))
	//fmt.Printf("\nindexes: %v\n",indexes2)


	start = time.Now()
	cmd = "SELECT word FROM words WHERE id IN (" + strings.Join(indexes2, ",") + ")"
	//fmt.Println(cmd)
	rows, err = db.Query(cmd)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var word string
		if err := rows.Scan(&word); err != nil {
			panic(err)
		}
		//fmt.Printf("%s\n", word)
		matches[numm] = word
		numm++
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}

	fmt.Printf("\nDatabase search 3 took %s \n", time.Since(start))
	matches = matches[0:numm]




	findings_leven = make([]int, N)
	findings_matches = make([]string, N)

	wg.Add(N)
	for i := 0; i < N; i++ {
		go search(matches[i*len(matches)/N:(i+1)*len(matches)/N], s, i)
	}
	wg.Wait()

	lowest := 100
	best_index := 0
	for i := 0; i < len(findings_leven); i++ {
		if findings_leven[i] < lowest {
			lowest = findings_leven[i]
			best_index = i
		}
	}

	return findings_matches[best_index], lowest
}

func getMatch(s string, path string) (string, int) {
	partials := getPartials(s)

	//fmt.Printf("Partials: %v", partials)
	runtime.GOMAXPROCS(8)
	N := 8

	indexMatches := getIndiciesFromPartial(partials, path)

	matches := ReadLines("cache/keys.list", indexMatches[1:])

	findings_leven = make([]int, N)
	findings_matches = make([]string, N)

	wg.Add(N)
	for i := 0; i < N; i++ {
		go search(matches[i*len(matches)/N:(i+1)*len(matches)/N], s, i)
	}
	wg.Wait()

	lowest := 100
	best_index := 0
	for i := 0; i < len(findings_leven); i++ {
		if findings_leven[i] < lowest {
			lowest = findings_leven[i]
			best_index = i
		}
	}

	return findings_matches[best_index], lowest
}

func search(matches []string, target string, process int) {
	defer wg.Done()
	match := "No match"
	target = strings.ToLower(target)
	bestLevenshtein := 1000
	for i := 0; i < len(matches); i++ {
		d := levenshtein.Distance(target, strings.ToLower(matches[i]))
		if d < bestLevenshtein {
			bestLevenshtein = d
			match = matches[i]
		}
	}
	findings_matches[process] = match
	findings_leven[process] = bestLevenshtein
}

func main() {
	dbPath = "./words.db"
	tuple_length = 3
	file_tuple_length = 3
	if strings.EqualFold(os.Args[1], "help") {
		fmt.Printf("Version 1.2 - %v-mer tuples, removing commons\n", tuple_length)
		fmt.Println("./match-concurrent build <NAME OF WORDLIST> - builds SQL file with word insertions\n")
		fmt.Println("./match-concurrent 'word or words to match' /directions/to/words.database\n")
	} else if strings.EqualFold(os.Args[1], "build") {
		os.Mkdir("cache", 0775)
		generateHash2(os.Args[2])

	} else {
		match, lowest := getMatch2(os.Args[1], os.Args[2])
		fmt.Printf("%v|||%v\n", match, lowest)
	}
}
