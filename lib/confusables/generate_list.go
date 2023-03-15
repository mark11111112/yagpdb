//go:build ignore

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"go/format"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"unicode"
)

var (
	ConfusablesURI       = "https://www.unicode.org/Public/security/revision-06/confusables.txt"
	ExtraConfusablesFile = "./extraConfusables.json"

	header = `// This file was generated by go generate; DO NOT EDIT
package %s
`
)

var BasicLatin = &unicode.RangeTable{
	R16: []unicode.Range16{
		{0x0021, 0x007E, 1},
	},
	R32:         []unicode.Range32{},
	LatinOffset: 5,
}

// Takes a character name as input and then verifies if it's in the list of allowed characters.
func isAllowed(from, to string) bool {
	fromRune := []rune(from)
	toRune := []rune(to)

	if len(fromRune) > 1 {
		return false
	}

	for _, rn := range fromRune {
		if unicode.In(rn, BasicLatin) {
			return false
		}
	}
	for _, rn := range toRune {
		if unicode.In(rn, BasicLatin) || unicode.IsSpace(rn) {
			return true
		}
	}

	return false
}

func formatUnicodeIDs(ids string) string {
	var formattedIDs string
	for _, charID := range strings.Split(ids, " ") {
		newID := fmt.Sprintf("\\U%s%s", strings.Repeat("0", 8-len(charID)), charID)
		formattedIDs += newID
	}

	return formattedIDs
}

func main() {
	var confusables = make(map[string]string)

	r := regexp.MustCompile(`(?i)(?P<sus>[a-zA-Z0-9 ]*) ;	(?P<unsus>[a-zA-Z0-9 ]*)+ ;	[a-z]{2,}	#\*? \( (?P<suschar>.+) →(?: .+ →)* (?P<unsuschar>.+) \) (?:.+)+ → (?:.+)`)

	// Add extra confusables as defined in extraConfusables.json.
	extraConfusables, err := os.OpenFile(ExtraConfusablesFile, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		fmt.Println(err)
		return
	}

	defer extraConfusables.Close()

	decoder := json.NewDecoder(extraConfusables)

	if err := decoder.Decode(&confusables); err != nil {
		fmt.Println(err)
	}

	// Fetch confusables from unicode.org.
	res, err := http.Get(ConfusablesURI)
	if err != nil {
		fmt.Println(err)
		return
	}

	defer res.Body.Close()

	scanner := bufio.NewScanner(res.Body)
	for scanner.Scan() {
		txt := scanner.Text()
		matches := r.FindStringSubmatch(txt)

		if len(matches) <= 0 {
			continue
		}

		// Checks if character is latin.
		if allowed := isAllowed(matches[3], matches[4]); !allowed {
			continue
		}

		// Converts unicode IDs into format \U<ID>.
		confusable := formatUnicodeIDs(matches[1])
		targettedCharacter := formatUnicodeIDs(matches[2])

		confusables[confusable] = targettedCharacter
	}

	if err := scanner.Err(); err != nil {
		fmt.Println(err)
		return
	}

	fileContent := "var confusables = []string{\n"

	for confusable, confused := range confusables {
		fileContent += fmt.Sprintf("	\"%s\",\"%s\",\n", confusable, confused)
	}

	fileContent += "}"

	WriteGoFile("confusables_table.go", "confusables", []byte(fileContent))
}

func WriteGoFile(filename, pkg string, b []byte) {
	w, err := os.Create(filename)
	if err != nil {
		log.Fatalf("Could not create file %s: %v", filename, err)
	}
	defer w.Close()

	_, err = fmt.Fprintf(w, header, pkg)
	if err != nil {
		log.Fatalf("Error writing header: %v", err)
	}

	// Strip leading newlines.
	for len(b) > 0 && b[0] == '\n' {
		b = b[1:]
	}
	formatted, err := format.Source(b)

	if err != nil {
		// Print the original buffer even in case of an error so that the
		// returned error can be meaningfully interpreted.
		w.Write(b)
		log.Fatalf("Error formatting file %s: %v", filename, err)
	}

	if _, err := w.Write(formatted); err != nil {
		log.Fatalf("Error writing file %s: %v", filename, err)
	}
}