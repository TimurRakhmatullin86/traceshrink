package costprocessor

import (
	_ "embed"
	"sync"
)

//go:embed model_prices.json
var builtinPricingData []byte

var (
	builtinOnce  sync.Once
	builtinTable PricingTable
	builtinErr   error
)

func BuiltinPricingTable() (PricingTable, error) {
	builtinOnce.Do(func() {
		builtinTable, builtinErr = NewPricingTable(builtinPricingData)
	})
	return builtinTable, builtinErr
}
