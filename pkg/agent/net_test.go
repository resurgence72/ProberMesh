package agent

import (
	"fmt"
	"testing"
)

func TestGetSelfRegion(t *testing.T) {
	tRegion := getSelfRegion("")
	fmt.Println("region ", tRegion)

}
