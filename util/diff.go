package util

// func DiffStringerLists(sources []fmt.Stringer, targets []fmt.Stringer) (inSource []fmt.Stringer, inTarget []fmt.Stringer, inBoth []fmt.Stringer) {
// 	var inBothMap map[string]fmt.Stringer
//
// firstLoop:
// 	for _, source := range sources {
// 		for _, target := range targets {
// 			if source.String() == target.String() {
// 				inBothMap[source.String()] = source
// 				continue firstLoop
// 			}
// 		}
//
// 		inSource = append(inSource, source)
// 	}
//
// secondLoop:
// 	for _, target := range targets {
// 		for _, source := range sources {
// 			if target.String() == source.String() {
// 				if _, found := inBothMap[target.String()]; !found {
// 					inBothMap[target.String()] = target
// 				}
// 				continue secondLoop
// 			}
// 		}
//
// 		inTarget = append(inTarget, target)
// 	}
//
// 	for _, ref := range inBothMap {
// 		inBoth = append(inBoth, ref)
// 	}
//
// 	return inSource, inTarget, inBoth
// }
