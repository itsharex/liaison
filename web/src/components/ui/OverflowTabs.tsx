import { useLayoutEffect, useRef, useState } from 'react';
import { useI18n } from '@/i18n';
import ActionMenu from './ActionMenu';
import './OverflowTabs.less';

type Item = { value: string; label: string };
export default function OverflowTabs({items,value,onChange,label}:{items:Item[];value:string;onChange:(value:string)=>void;label:string}) {
  const {tr}=useI18n();
  const root=useRef<HTMLDivElement>(null), measure=useRef<HTMLDivElement>(null);
  const [visible,setVisible]=useState<string[]>([]);
  const signature=JSON.stringify(items);
  useLayoutEffect(()=>{
    let disposed=false;
    const update=()=>{
      if(disposed||!root.current||!measure.current)return;
      const widths=Array.from(measure.current.children).map(el=>el.getBoundingClientRect().width);
      const available=root.current.clientWidth;
      if(widths.reduce((sum,w)=>sum+w+8,0)-8<=available){setVisible(items.map(item=>item.value));return;}
      const budget=Math.max(0,available-40), chosen:number[]=[];
      let used=0;
      for(let i=0;i<items.length;i++){if(used+widths[i]>budget)break;chosen.push(i);used+=widths[i]+8;}
      const selected=items.findIndex(item=>item.value===value);
      if(selected>=0&&!chosen.includes(selected)){
        while(chosen.length&&used+widths[selected]>budget){const last=chosen.pop()!;used-=widths[last]+8;}
        chosen.push(selected);
      }
      setVisible(chosen.sort((a,b)=>a-b).map(i=>items[i].value));
    };
    update();
    const observer=new ResizeObserver(update);if(root.current)observer.observe(root.current);if(measure.current)observer.observe(measure.current);
    void document.fonts.ready.then(update);
    return()=>{disposed=true;observer.disconnect();};
  },[signature,value]);
  const hidden=items.filter(item=>!visible.includes(item.value));
  return <div className="liaison-overflow-tabs" ref={root}>
    <div ref={measure} className="liaison-overflow-tabs-measure" aria-hidden="true">{items.map(item=><span className="liaison-overflow-tab" key={item.value}>{item.label}</span>)}</div>
    <nav aria-label={label}>{items.filter(item=>visible.includes(item.value)).map(item=><button type="button" className="liaison-overflow-tab" key={item.value} aria-pressed={value===item.value} onClick={()=>onChange(item.value)}>{item.label}</button>)}</nav>
    {!!hidden.length&&<ActionMenu vertical label={tr('更多协议','More protocols')} items={hidden.map(item=>({label:item.label,onClick:()=>{onChange(item.value);requestAnimationFrame(()=>root.current?.querySelector<HTMLButtonElement>('[aria-pressed="true"]')?.focus());}}))}/>}
  </div>;
}
