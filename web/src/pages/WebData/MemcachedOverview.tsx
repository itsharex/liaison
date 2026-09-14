import {useEffect, useState, type ChangeEvent} from 'react';
import {RefreshCw} from 'lucide-react';
import {Button, Input, Typography} from '@/components/ui/complex';
import {useI18n} from '@/i18n';
import {executeWebDataStatement} from '@/services/api';

export default function MemcachedOverview({token, keyValue, onKeyChange}: {
  token: string; keyValue: string; onKeyChange: (value: string) => void;
}) {
  const {tr} = useI18n();
  const [revision, setRevision] = useState(0);
  const [stats, setStats] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(true);
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    let active = true;
    setLoading(true); setFailed(false); setStats({});
    executeWebDataStatement(token, JSON.stringify({operation: 'stats'})).then(res => {
      if (!active) return;
      if (res.code !== 200 || !res.data || res.data.error) throw new Error('stats');
      setStats(Object.fromEntries((res.data.rows || []).map(row => [String(row.name), String(row.value)])));
    }).catch(() => {if (active) setFailed(true);})
      .finally(() => {if (active) setLoading(false);});
    return () => {active = false;};
  }, [token, revision]);
  const number = (name: string) => stats[name] !== undefined && Number.isFinite(Number(stats[name])) ? Number(stats[name]) : NaN;
  const bytes = (value: number) => !Number.isFinite(value) ? '—' : value >= 1048576 ? `${(value / 1048576).toFixed(1)} MiB` : `${value.toLocaleString()} B`;
  const uptime = number('uptime');
  const hits = number('get_hits'), misses = number('get_misses');
  const metrics = [
    [tr('版本', 'Version'), stats.version || '—'],
    [tr('运行时间', 'Uptime'), Number.isFinite(uptime) ? `${Math.floor(uptime / 86400)}${tr('天', 'd')} ${Math.floor(uptime % 86400 / 3600)}${tr('时', 'h')} ${Math.floor(uptime % 3600 / 60)}${tr('分', 'm')}` : '—'],
    [tr('内存 / 上限', 'Memory / limit'), `${bytes(number('bytes'))} / ${bytes(number('limit_maxbytes'))}`],
    [tr('当前键数', 'Current keys'), Number.isFinite(number('curr_items')) ? number('curr_items').toLocaleString() : '—'],
    [tr('累计命中率', 'Overall hit rate'), hits + misses > 0 ? `${(hits / (hits + misses) * 100).toFixed(1)}%` : '—'],
  ];
  return <>
    <div className="webdata-side-header">
      <Typography.Text strong>{tr('缓存概况', 'Cache overview')}</Typography.Text>
      <Button className="webdata-refresh-button" size="small" aria-label={tr('刷新概况', 'Refresh overview')} icon={<RefreshCw />} loading={loading} onClick={() => setRevision(value => value + 1)} />
    </div>
    <div className="webdata-cache-overview" aria-busy={loading}>
      {failed ? <p role="alert">{tr('概况加载失败，请刷新重试。', 'Could not load overview. Refresh to retry.')}</p> : <dl>{metrics.map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{loading ? '—' : value}</dd></div>)}</dl>}
    </div>
    <div className="webdata-cache-key">
      <label htmlFor="memcached-key">{tr('Key 查询', 'Key lookup')}</label>
      <Input id="memcached-key" aria-label="Key" placeholder={tr('输入已知 Key', 'Enter a known key')} value={keyValue} onChange={(event: ChangeEvent<HTMLInputElement>) => onKeyChange(event.target.value)} />
      <p>{tr('Memcached 不提供完整键目录。输入 Key 后，在右侧选择操作。', 'Memcached has no full key catalog. Enter a key, then select an action on the right.')}</p>
    </div>
  </>;
}
