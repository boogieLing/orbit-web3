"use client";

import type { InteractionRecord } from "./api";
import { dateLabel } from "./api";

// 按当前应用展示可选记录，选择与说明共用同一份服务端数据。
export default function InteractionPicker({ records, appID, value, onChange, disabled = false }: { records: InteractionRecord[]; appID: string; value: string; onChange: (value: string) => void; disabled?: boolean }) {
  const choices = records.filter(record => record.app_id === appID);
  const selected = choices.find(record => record.id === value);
  return <div className="interaction-picker"><label className="field">选择交互记录<select value={value} disabled={disabled || !appID} onChange={event => onChange(event.target.value)}><option value="">{appID ? "暂不关联记录" : "先选择应用"}</option>{choices.map(record => <option key={record.id} value={record.id}>{record.title} · {dateLabel(record.occurred_at)}</option>)}</select></label>{selected && <p>{selected.detail}</p>}<small>将操作摘要和使用体验放在一起，方便回顾。</small></div>;
}
