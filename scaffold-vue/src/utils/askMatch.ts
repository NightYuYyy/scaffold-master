/* 本地规则问答的匹配与权限码解析（纯函数），不调用 AI。 */
export interface IntentPattern { id: string; patterns: RegExp[] }

/** 返回第一个命中的意图 ID；都不命中返回 null（交给关键词检索） */
export function matchIntent(question: string, intents: IntentPattern[]) {
  const q = question.trim()
  if (!q) return null
  return intents.find(intent => intent.patterns.some(p => p.test(q)))?.id ?? null
}

const VERBS: Array<[RegExp, string]> = [
  [/删除|删掉|移除/, 'remove'],
  [/新增|添加|创建|新建/, 'add'],
  [/修改|编辑|更新/, 'edit'],
  [/导出/, 'list'],
  [/重置密码/, 'resetPwd'],
  [/调整|加减|充值|扣减/, 'adjust'],
  [/查看|查询|访问|看到/, 'list'],
]
const OBJECTS: Array<[RegExp, string]> = [
  [/应用用户|应用的用户|C ?端/, 'platform:user'],
  [/密钥/, 'platform:appkey'],
  [/字段/, 'platform:field'],
  [/认证日志/, 'platform:authlog'],
  [/流水|数值账户|账户/, 'platform:counter'],
  [/应用/, 'platform:app'],
  [/后台账号|用户|账号/, 'system:user'],
  [/角色/, 'system:role'],
  [/菜单/, 'system:menu'],
  [/字典/, 'system:dict'],
  [/参数|配置/, 'system:config'],
  [/操作日志/, 'monitor:operlog'],
  [/登录日志/, 'monitor:loginlog'],
  [/代码生成/, 'tool:gen'],
]

/** 「谁能删除后台账号」→ system:user:remove，「谁能调整账户」→ platform:counter:adjust；问题里直接写了权限码则原样返回 */
export function permissionFromQuestion(question: string) {
  const code = question.match(/[a-z]+:[a-z]+:[a-zA-Z]+/)
  if (code) return code[0]
  const verb = VERBS.find(([re]) => re.test(question))?.[1]
  const object = OBJECTS.find(([re]) => re.test(question))?.[1]
  return verb && object ? `${object}:${verb}` : null
}
