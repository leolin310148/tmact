import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { onPointerDownNoBlur } from "../lib/dom";
import { loadAgentUsageHistory } from "../api/client";
import type { AgentUsageHistory } from "../types/server";

export default function UsageTrendDialog({ onClose }: { onClose: () => void }) {
  const [data, setData] = useState<AgentUsageHistory | null>(null);
  const [error, setError] = useState("");
  const [days, setDays] = useState(7);
  const [machine, setMachine] = useState("all");
  const [provider, setProvider] = useState("all");
  const dialog = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const invoking = document.activeElement;
    const el = dialog.current;
    el?.showModal();
    const controller = new AbortController();
    void loadAgentUsageHistory(controller.signal)
      .then(({ res, data: history }) => {
        if (!res.ok || !Array.isArray(history.machines)) throw new Error("無法取得趨勢，請稍後重試");
        setData(history);
      }).catch((err: unknown) => { if (!controller.signal.aborted) setError(err instanceof Error ? err.message : "無法取得趨勢"); });
    return () => { controller.abort(); el?.close(); if (invoking instanceof HTMLElement && invoking.isConnected) invoking.focus(); };
  }, []);
  const dates = Array.from({ length: days }, (_, i) => {
    const date = new Date(); date.setUTCDate(date.getUTCDate() - days + 1 + i);
    return date.toISOString().slice(0, 10);
  });
  const machines = (data?.machines || []).filter((m) => machine === "all" || m.machine === machine);
  const providers = [...new Set((data?.machines || []).flatMap((m) => [...m.daily.map((d) => d.provider), ...m.usage.map((u) => u.provider)]))].sort();
  const daily = dates.map((date) => ({ date, usd: machines.reduce((sum, m) => sum + m.daily.filter((d) => d.date === date && (provider === "all" || d.provider === provider)).reduce((n, d) => n + d.usd, 0), 0) }));
  const maxCost = Math.max(1, ...daily.map((d) => d.usd));
  const start = new Date(dates[0] + "T00:00:00Z").getTime();
  const end = Date.now();
  return createPortal(
    <dialog ref={dialog} className="usage-trend-dialog" aria-labelledby="usage-trend-title" onCancel={(e) => { e.preventDefault(); onClose(); }} onClick={(e) => { if (e.target === dialog.current) { const r = dialog.current.getBoundingClientRect(); if (e.clientX < r.left || e.clientX > r.right || e.clientY < r.top || e.clientY > r.bottom) onClose(); } }}>
      <div className="settings-head"><span id="usage-trend-title">Cost / Usage 趨勢</span><button type="button" className="settings-close" aria-label="關閉趨勢" onPointerDown={onPointerDownNoBlur} onClick={onClose}>×</button></div>
      <div className="usage-trend-filters">
        <label>期間 <select value={days} onChange={(e) => setDays(Number(e.target.value))}><option value={7}>近 7 天</option><option value={30}>近 30 天</option></select></label>
        <label>機器 <select value={machine} onChange={(e) => setMachine(e.target.value)}><option value="all">全部機器</option>{data?.machines.map((m) => <option key={m.machine} value={m.machine}>{m.machine}</option>)}</select></label>
        <label>Provider <select value={provider} onChange={(e) => setProvider(e.target.value)}><option value="all">全部</option>{providers.map((p) => <option key={p}>{p}</option>)}</select></label>
      </div>
      {!data && !error ? <p role="status">載入趨勢中…首次需掃描各機器的 session 紀錄，可能需要一些時間。</p> : null}
      {error ? <p role="alert">{error}</p> : null}
      {data ? <>
        {machines.filter((m) => m.error).map((m) => <p role="status" key={m.machine}>{m.machine}：{m.error}（總計可能不完整）</p>)}
        <h3>每日 Cost · ${daily.reduce((sum, d) => sum + d.usd, 0).toFixed(2)} USD</h3>
        <p className="usage-trend-note">API 價格換算估值，非實際帳單。日期以 UTC 計算；無紀錄日期顯示 $0。</p>
        <svg className="usage-trend-chart" viewBox="0 0 720 190" role="img" aria-label="每日 cost 長條圖">
          {daily.map((d, i) => <g key={d.date}><title>{d.date}: ${d.usd.toFixed(2)}</title><rect x={i * 700 / days + 10} y={155 - d.usd / maxCost * 140} width={Math.max(2, 700 / days - 5)} height={Math.max(1, d.usd / maxCost * 140)} fill="var(--green)"/><text x={i * 700 / days + 10} y={177} fill="currentColor" fontSize="11">{days === 7 || i % 5 === 0 ? d.date.slice(5) : ""}</text></g>)}
        </svg>
        <details><summary>每日金額明細</summary><table className="usage-trend-table"><thead><tr><th>日期 UTC</th><th>USD</th></tr></thead><tbody>{daily.map((d) => <tr key={d.date}><td>{d.date}</td><td>${d.usd.toFixed(2)}</td></tr>)}</tbody></table></details>
        <h3>Usage · 已使用額度 %</h3>
        <p className="usage-trend-note">從啟用記錄後開始累積。各機器分開顯示；重設與超過 20 分鐘的缺口不連線。</p>
        {machines.map((m) => {
          const samples = m.usage.filter((u) => new Date(u.at).getTime() >= start && (provider === "all" || u.provider === provider));
          const keys = [...new Set(samples.flatMap((u) => u.windows.map((w) => u.provider + "/" + w.name)))].sort();
          return <section key={m.machine}><h4>{m.machine}</h4>{keys.length === 0 ? <p>尚無 usage 歷史紀錄</p> : keys.map((key) => {
            const points = samples.flatMap((u) => u.windows.filter((w) => u.provider + "/" + w.name === key).map((w) => ({ at: new Date(u.at).getTime(), value: w.used_percent, reset: w.resets_at })) ).sort((a, b) => a.at - b.at);
            return <div key={key}><div>{key}</div><svg className="usage-trend-chart" viewBox="0 0 720 150" role="img" aria-label={m.machine + " " + key + " 已使用額度趨勢"}>
              {[0, 50, 100].map((v) => <g key={v}><text x="0" y={130 - v * 1.1} fill="currentColor" fontSize="11">{v}%</text><line x1="35" x2="710" y1={126 - v * 1.1} y2={126 - v * 1.1} stroke="currentColor" opacity=".12"/></g>)}
              {points.map((p, i) => {
                const x = 35 + (p.at - start) / Math.max(1, end - start) * 675;
                const y = 126 - Math.min(100, Math.max(0, p.value)) * 1.1;
                const prev = points[i - 1];
                return <g key={i}><title>{new Date(p.at).toLocaleString()} · {p.value}%</title>{prev && prev.reset === p.reset && p.at - prev.at <= 20 * 60000 ? <line x1={35 + (prev.at - start) / Math.max(1, end - start) * 675} y1={126 - Math.min(100, Math.max(0, prev.value)) * 1.1} x2={x} y2={y} stroke="var(--green)" strokeWidth="2"/> : null}<circle cx={x} cy={y} r="1.8" fill="var(--green)"/></g>;
              })}<text x="35" y="147" fill="currentColor" fontSize="11">{dates[0]}</text><text x="630" y="147" fill="currentColor" fontSize="11">{dates.at(-1)}</text>
            </svg></div>;
          })}</section>;
        })}
      </> : null}
    </dialog>, document.body,
  );
}
