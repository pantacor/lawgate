package shop

import "fmt"

// Pay records a payment the balance covers.
func Pay(amount, balance int) error {
	if !affordable(amount, balance) {
		return fail("SHOP_JOB 1 over balance")
	}
	return record("SHOP_JOB 2 recorded")
}

func affordable(amount, balance int) bool {
	return amount <= balance
}

func fail(tag string) error {
	return fmt.Errorf("%s", tag)
}

func record(tag string) error {
	return nil
}
