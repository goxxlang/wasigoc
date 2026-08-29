// Exercises time.Time/Duration -- excludes time.Now() (non-deterministic,
// can't be a golden's exact-match expected output).
package main

import (
	"fmt"
	"time"
)

func main() {
	t := time.Unix(1700000000, 500000000)
	fmt.Println(t.Unix())
	fmt.Println(t.Year(), t.Month(), t.Day())
	fmt.Println(t.Hour(), t.Minute(), t.Second())
	fmt.Println(t.Weekday())
	fmt.Println(t.String())

	t2 := t.Add(90 * time.Minute)
	fmt.Println(t2.Sub(t))
	fmt.Println(t2.After(t), t.Before(t2), t.Equal(t))

	fmt.Println(time.FormatDuration(time.Duration(1500000000)))
	fmt.Println(time.FormatDuration(250 * time.Millisecond))
	fmt.Println(time.FormatDuration(3 * time.Hour))
	fmt.Println(time.FormatDuration(time.Duration(0)))
}
