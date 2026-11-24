package shop

import "testing"

// TestRefusesOverBalance proves never-overspend.
func TestRefusesOverBalance(t *testing.T) {
	if Pay(5, 3) == nil {
		t.Fatal("a payment over the balance was recorded")
	}
	if Pay(3, 3) != nil {
		t.Fatal("a payment the balance covers was refused")
	}
}
