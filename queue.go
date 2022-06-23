package runner

import "fmt"

func GetResultsQueue(workQueue string) string {
	return fmt.Sprintf("%v:results", workQueue)
}
