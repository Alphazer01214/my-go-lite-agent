package plugin

import "fmt"

// AutostartClosure 选出启动时要挂载的插件集合与顺序。
// 根 = autostart:true 的清单；对 depends_on 做 DFS 后序，保证依赖先于依赖方。
// 缺失依赖与依赖环按软失败处理（protocol §6）：记 warn 并继续，宿主不退出。
func AutostartClosure(discs Discoveries) (ordered []Discovery, warns []string) {
	byName := make(map[string]Discovery, len(discs.Plugins))
	var roots []string
	for _, d := range discs.Plugins {
		name := d.Manifest.Name
		if name == "" {
			warns = append(warns, fmt.Sprintf("skip plugin in %s: empty manifest name", d.Dir))
			continue
		}
		if _, dup := byName[name]; dup {
			warns = append(warns, fmt.Sprintf("skip duplicate plugin %s in %s", name, d.Dir))
			continue
		}
		byName[name] = d
		if d.Manifest.Autostart {
			roots = append(roots, name)
		}
	}

	// state: 0 未访问 / 1 在 DFS 栈上 / 2 已输出
	state := make(map[string]int, len(byName))
	var stack []string
	var visit func(name, requiredBy string)
	visit = func(name, requiredBy string) {
		switch state[name] {
		case 2:
			return
		case 1:
			warns = append(warns, fmt.Sprintf("dependency cycle: %v -> %s", stack, name))
			return
		}
		d, ok := byName[name]
		if !ok {
			where := "startup root"
			if requiredBy != "" {
				where = "required by " + requiredBy
			}
			warns = append(warns, fmt.Sprintf("missing dependency %q (%s)", name, where))
			return
		}
		state[name] = 1
		stack = append(stack, name)
		for _, dep := range d.Manifest.DependsOn {
			visit(dep, name)
		}
		stack = stack[:len(stack)-1]
		state[name] = 2
		ordered = append(ordered, d)
	}

	for _, name := range roots {
		visit(name, "")
	}
	return ordered, warns
}
