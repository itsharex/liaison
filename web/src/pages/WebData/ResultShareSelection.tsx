import {useState} from 'react';
import {Button} from '@/components/ui';
import {useI18n} from '@/i18n';
import {resultSharePreview} from './resultShare';

// A fixed browser-result snapshot. Selection never issues a query or model request.
export default function ResultShareSelection({result,protocol,onChange}:{result:API.WebDataExecuteResult;protocol:string;onChange:(text:string)=>void}) {
  const {tr}=useI18n();
  const source=result.rows || [];
  const columns=[...new Set(source.flatMap(row=>Object.keys(row)))];
  const [rows,setRows]=useState<number[]>(source.slice(0,20).map((_,i)=>i));
  const [fields,setFields]=useState<string[]>(columns.slice(0,10));
  const [page,setPage]=useState(0);
  const update=(nextRows:number[],nextFields:string[])=>{
    setRows(nextRows);setFields(nextFields);
    onChange(nextRows.length&&nextFields.length?resultSharePreview(result,protocol,{rows:nextRows,columns:nextFields}):'');
  };
  return <details className="result-share-selection" style={{marginBottom:16,accentColor:'rgb(var(--accent))'}}>
    <summary>{tr('选择行和列', 'Choose rows and columns')} · {rows.length}/20 {tr('行','rows')} · {fields.length}/10 {tr('列','columns')}</summary>
    <p>{tr('默认选择前 20 行和前 10 列。更改选择会重新生成下方片段，覆盖片段编辑。', 'Defaults to the first 20 rows and 10 columns. Changing selection regenerates the sample and replaces sample edits.')}</p>
    <div style={{display:'flex',flexWrap:'wrap',gap:12}}>{columns.map(column=><label key={column} style={{display:'flex',alignItems:'center',gap:6,maxWidth:'100%',overflowWrap:'anywhere'}}><input type="checkbox" checked={fields.includes(column)} disabled={!fields.includes(column)&&fields.length>=10} onChange={()=>update(rows,fields.includes(column)?fields.filter(c=>c!==column):[...fields,column])}/>{column}</label>)}</div>
    <Button style={{marginTop:12}} onClick={()=>update([],fields)}>{tr('清空行选择','Clear row selection')}</Button>
    <div style={{maxHeight:160,overflow:'auto',marginTop:12}}>
      <table style={{width:'100%',fontSize:12}}><thead><tr><th>{tr('选择','Select')}</th><th>{tr('行号','Row')}</th><th>{tr('内容预览','Preview')}</th></tr></thead><tbody>
        {source.slice(page*20,page*20+20).map((row,offset)=>{const index=page*20+offset;return <tr key={index}>
          <td><input type="checkbox" aria-label={`${tr('行','Row')} ${index+1}`} checked={rows.includes(index)} disabled={!rows.includes(index)&&rows.length>=20} onChange={()=>update(rows.includes(index)?rows.filter(i=>i!==index):[...rows,index],fields)}/></td><td>{index+1}</td>
          <td style={{overflowWrap:'anywhere',fontFamily:'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace'}}>{JSON.stringify(JSON.parse(resultSharePreview({type:'rows',rows:[row]},protocol,{rows:[0],columns:fields})).rows[0] || {}).slice(0,180)}</td>
        </tr>;})}
      </tbody></table>
    </div>
    <div style={{display:'flex',gap:12,alignItems:'center',marginTop:12}}><Button disabled={page===0} onClick={()=>setPage(page-1)}>{tr('上一页','Previous')}</Button><span>{page+1}/{Math.max(1,Math.ceil(source.length/20))}</span><Button disabled={(page+1)*20>=source.length} onClick={()=>setPage(page+1)}>{tr('下一页','Next')}</Button></div>
  </details>;
}
