package service

import "math/rand"

// The demo workspace is an Apple storefront: this catalogue names the product
// pages the generated traffic lands on and prices the conversions those visits
// produce. A contact's purchase and the session that produced it read the same
// row, so they can never disagree about what a MacBook Pro costs — the demo has
// no orders from anywhere else to reconcile with.
//
// Prices are the US list prices the Staminads demo fixtures use.
type demoProduct struct {
	Name     string
	MinPrice float64
	MaxPrice float64
}

var demoAppleProducts = []demoProduct{
	{"iPhone 17 Pro", 999, 1199},
	{"iPhone Air", 999, 999},
	{"iPhone 17", 799, 799},
	{"iPhone 16e", 599, 599},
	{"MacBook Air", 999, 1299},
	{"MacBook Pro", 1599, 2499},
	{"iMac", 1299, 1299},
	{"Mac mini", 599, 599},
	{"Mac Studio", 1999, 3999},
	{"Mac Pro", 5999, 6999},
	{"iPad Pro", 999, 1299},
	{"iPad Air", 599, 599},
	{"iPad mini", 499, 499},
	{"Apple Watch Series 11", 399, 399},
	{"Apple Watch Ultra 3", 799, 799},
	{"Apple Watch SE 3", 249, 249},
	{"AirPods Pro", 249, 249},
	{"AirPods 4", 129, 129},
	{"AirPods Max", 549, 549},
	{"Apple TV 4K", 129, 149},
	{"HomePod mini", 99, 99},
}

// demoPriceFor prices one product. Configurable models are priced in $100 steps
// the way Apple's own tiers are, except for ranges narrower than a step, which
// would otherwise always collapse onto the minimum.
//
// The source is required, not optional: every price in the demo belongs to a
// generated session, and a draw from the global source would make the same reset
// produce different revenue every time.
func demoPriceFor(product demoProduct, rng *rand.Rand) float64 {
	span := product.MaxPrice - product.MinPrice

	switch {
	case span <= 0:
		return product.MinPrice
	case span < 100:
		return product.MinPrice + float64(rng.Intn(int(span)+1))
	default:
		return product.MinPrice + float64(rng.Intn(int(span/100)+1))*100
	}
}
