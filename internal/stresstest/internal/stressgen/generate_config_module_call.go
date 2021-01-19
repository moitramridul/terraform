package stressgen

import (
	"math/rand"

	"github.com/hashicorp/terraform/addrs"
	"github.com/zclconf/go-cty/cty"
)

// GenerateConfigModuleCall uses the given random number generator to generate
// a random ConfigModuleCall object.
func GenerateConfigModuleCall(rnd *rand.Rand, parentNS *Namespace) *ConfigModuleCall {
	addr := addrs.ModuleCall{Name: parentNS.GenerateShortName(rnd)}
	childNS := parentNS.ChildNamespace(addr.Name)
	ret := &ConfigModuleCall{
		Addr:           addr,
		Arguments:      make(map[addrs.InputVariable]ConfigExpr),
		ChildNamespace: childNS,
	}

	// We support all three of the repetition modes for modules here: for_each
	// over a map, count with a number, and single-instance mode. However,
	// the rest of our generation strategy here works only with strings and
	// so we need to do some trickery here to produce suitable inputs for
	// the repetition arguments while still having them generate references
	// sometimes, because the repetition arguments play an important role in
	// constructing the dependency graph.
	// We achieve this as follows:
	// - for for_each, we generate a map with a random number of
	//   randomly-generated keys where each of the values is an expression
	//   randomly generated in our usual way.
	// - for count, we generate a random expression in the usual way, assume
	//   that the result will be convertable to a string (because that's our
	//   current standard) and apply some predictable string functions to it
	//   in order to deterministically derive a number.
	// Both cases therefore allow for the meta-argument to potentially depend
	// on other objects in the configuration, even though our current model
	// only allows for string dependencies directly.

	const (
		chooseSingleInstance int = 0
		chooseForEach        int = 1
		chooseCount          int = 2
	)
	which := decideIndex(rnd, []int{
		chooseSingleInstance: 4,
		chooseForEach:        2,
		chooseCount:          2,
	})
	switch which {
	case chooseSingleInstance:
		// Nothing special to do, then. ForEachExpr and CountExpr will both
		// be nil.
	case chooseForEach:
		// We need to generate some randomly-selected instance keys, and then
		// associate each one with a randomly-selected expression.
		n := rnd.Intn(9)
		forEach := &ConfigExprForEach{
			Exprs: make(map[string]ConfigExpr, n),
		}
		for i := 0; i < n; i++ {
			k := parentNS.GenerateShortModifierName(rnd)
			expr := parentNS.GenerateExpression(rnd)
			forEach.Exprs[k] = expr
		}
		ret.ForEachExpr = forEach
	case chooseCount:
		// We need to randomly select a source expression and then wrap it
		// in our special ConfigExprCount type to make it appear as a
		// randomly-chosen small integer instead of a string.
		expr := parentNS.GenerateExpression(rnd)
		ret.CountExpr = &ConfigExprCount{Expr: expr}
	default:
		// This suggests either a bug in decideIndex or in our call
		// to decideIndex.
		panic("invalid decision")
	}

	objCount := rnd.Intn(25)
	objs := make([]ConfigObject, 0, objCount+1) // +1 for the boilerplate object

	// We always need a boilerplate object.
	boilerplate := &ConfigBoilerplate{
		ModuleAddr: childNS.ModuleAddr,
		Providers: map[string]addrs.Provider{
			"stressful": addrs.MustParseProviderSourceString("terraform.io/stresstest/stressful"),
		},
	}
	objs = append(objs, boilerplate)

	for i := 0; i < objCount; i++ {
		obj := GenerateConfigObject(rnd, childNS)
		objs = append(objs, obj)

		if cv, ok := obj.(*ConfigVariable); ok && cv.CallerWillSet {
			// The expression comes from parentNS here because the arguments
			// are defined in the calling module, not the called module.
			chosenExpr := parentNS.GenerateExpression(rnd)
			ret.Arguments[cv.Addr] = chosenExpr
		}
	}

	ret.Objects = objs

	declareConfigModuleCall(ret, childNS)
	return ret
}

// declareConfigModuleCall creates the declaration of the given module call in
// the given namespace. This is shared by both GenerateConfigModuleCall and by
// ConfigModuleCall.GenerateModified.
func declareConfigModuleCall(mc *ConfigModuleCall, ns *Namespace) {
	// In the case were we're generating a count expression, we can't know
	// until instantiation how many instances there will be, so we don't
	// declare any referencables in that case. That's not ideal, but we
	// accept the compromise because we can still generate references for
	// for_each and those two mechanisms share a lot of supporting code
	// in common. Having the number of instances for count be able to vary
	// between instantiations is also an interesting thing to test, even
	// though we can't guarantee to generate valid references in that case.
	if mc.CountExpr != nil {
		return
	}

	switch {
	case mc.ForEachExpr != nil:
		for keyStr := range mc.ForEachExpr.Exprs {
			for name := range mc.ChildNamespace.OutputValues {
				moduleInstAddr := addrs.ModuleCallInstance{
					Call: mc.Addr,
					Key:  addrs.StringKey(keyStr),
				}
				ref := NewConfigExprRef(moduleInstAddr, cty.GetAttrPath(name))
				ns.DeclareReferenceable(ref)
			}
		}
	default:
		for name := range mc.ChildNamespace.OutputValues {
			moduleInstAddr := addrs.ModuleCallInstance{
				Call: mc.Addr,
				Key:  addrs.NoKey,
			}
			ref := NewConfigExprRef(moduleInstAddr, cty.GetAttrPath(name))
			ns.DeclareReferenceable(ref)
		}
	}
}
