// 菜单权限判断工具：GVA 动态菜单为嵌套树（routerStore.rootMenus），
// 业务入口按钮按「角色是否有某菜单」控制显隐（如 AI 助手悬浮窗）。
export const menuHasName = (menus, name) => {
  if (!Array.isArray(menus)) return false
  return menus.some((m) => m.name === name || menuHasName(m.children, name))
}
