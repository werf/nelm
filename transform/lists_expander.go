package transform

// func NewListsExpander() LocalTransformer {
// 	return &ListsExpander{}
// }
//
// type ListsExpander struct{}
//
// func (f *ListsExpander) LocalTransform(opts LocalTransformOptions, resources ...kuberesource.LocalKubeResourcer) ([]kuberesource.LocalKubeResourcer, error) {
// 	var result []kuberesource.LocalKubeResourcer
//
// 	for _, res := range resources {
// 		if opts.SelectorFn != nil {
// 			if selected, err := opts.SelectorFn(res); err != nil {
// 				return nil, fmt.Errorf("failed to select resource: %w", err)
// 			} else if !selected {
// 				result = append(result, res)
// 				continue
// 			}
// 		}
//
// 		if !res.Unstructured().IsList() {
// 			result = append(result, res)
// 			continue
// 		}
//
// 		if err := res.Unstructured().EachListItem(
// 			func(obj runtime.Object) error {
// 				var err error
// 				unstruct := &unstructured.Unstructured{}
// 				unstruct.Object, err = runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
// 				if err != nil {
// 					return fmt.Errorf("error converting object to unstructured: %w", err)
// 				}
//
// 				result = append(result, kuberesource.NewLocalKubeResource(unstruct))
//
// 				return nil
// 			},
// 		); err != nil {
// 			return nil, fmt.Errorf("error iterating over resource list: %w", err)
// 		}
// 	}
//
// 	return result, nil
// }
