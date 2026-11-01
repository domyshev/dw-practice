import { configureStore, createSlice, type PayloadAction } from '@reduxjs/toolkit';
import { useDispatch, useSelector } from 'react-redux';
import {requestJSON} from './http.ts';
import {parseChartPeriod,type ChartPeriod} from './activity.ts';
function storedPeriod():ChartPeriod{try{return parseChartPeriod(localStorage.getItem('dw-chart-period'))}catch{return 'week'}}
export type Practice = 'prostrations' | 'diamond-mind' | 'mandala' | 'guru-yoga';
export type Entry = {id:string;userId:string;practice:Practice;large:number;small:number;repetitions:number;total:number;initial:boolean;occurredAt:string;timeZone:string;offsetMinutes:number;note:string;createdAt:string;updatedAt:string;version:number};
export type Audit = {id:string;entryId:string;action:string;at:string;before:Entry|null;after:Entry|null};
export type User = {id:string;email:string};
const slice = createSlice({
  name: 'practice',
  initialState: {user:null as User|null, entries:[] as Entry[], audit:[] as Audit[], view:'overview', filter:'all', chartPeriod:storedPeriod(), sessionGeneration:0, loadId:0, refreshError:'', revision:0},
  reducers: {
    signedIn(s, a:PayloadAction<User>) {
      s.user=a.payload; s.entries=[]; s.audit=[]; s.view='overview'; s.filter='all'; s.refreshError=''; s.sessionGeneration++; s.loadId=0;
    },
    signedOut(s) {
      s.user=null; s.entries=[]; s.audit=[]; s.view='overview'; s.filter='all'; s.refreshError=''; s.sessionGeneration++; s.loadId=0;
    },
    loading(s, a:PayloadAction<{generation:number; loadId:number}>) {
      if (s.sessionGeneration===a.payload.generation) {s.loadId=a.payload.loadId; s.refreshError='';}
    },
    loaded(s, a:PayloadAction<{generation:number;loadId:number;entries:Entry[];audit:Audit[]}>) {
      if (s.sessionGeneration!==a.payload.generation || s.loadId!==a.payload.loadId) return;
      s.entries=a.payload.entries; s.audit=a.payload.audit; s.refreshError=''; s.revision++;
    },
    loadFailed(s, a:PayloadAction<{generation:number;loadId:number;message:string}>) {
      if (s.sessionGeneration===a.payload.generation && s.loadId===a.payload.loadId) s.refreshError=a.payload.message;
    },
    committed(s, a:PayloadAction<{generation:number;id:string;entry:Entry|null}>) {
      if(s.sessionGeneration!==a.payload.generation) return;
      s.revision++;
      // Invalidate loads started before the mutation committed.
      s.loadId=0;
    },
    setChartPeriod:(s,a:PayloadAction<ChartPeriod>)=>{s.chartPeriod=parseChartPeriod(a.payload)},
    navigate:(s,a:PayloadAction<string>)=>{s.view=a.payload;s.filter='all'},
    filter:(s,a:PayloadAction<string>)=>{s.filter=a.payload}
  }
});
export const actions=slice.actions;
export const store=configureStore({reducer:slice.reducer});
export const useAppDispatch=useDispatch.withTypes<typeof store.dispatch>();
export const useAppSelector=useSelector.withTypes<ReturnType<typeof store.getState>>();
export class SessionChangedError extends Error {
  constructor(){super('The active session changed');this.name='SessionChangedError';}
}
const pending=new Set<AbortController>();
let observedGeneration=store.getState().sessionGeneration;
let savedPeriod=store.getState().chartPeriod;
store.subscribe(()=>{
  const period=store.getState().chartPeriod;
  if(period!==savedPeriod){savedPeriod=period;try{localStorage.setItem('dw-chart-period',period)}catch{}}
  const generation=store.getState().sessionGeneration;
  if(generation!==observedGeneration){observedGeneration=generation;for(const controller of pending)controller.abort();pending.clear();}
});
export async function api<T>(path:string,method='GET',body?:unknown,idempotencyKey?:string):Promise<T>{
  const generation=store.getState().sessionGeneration;
  const controller=new AbortController();pending.add(controller);
  try {
    const r=await requestJSON(path,{method,signal:controller.signal,
      headers:{...(method!=='GET'?{'Content-Type':'application/json'}:{}),...(idempotencyKey?{'Idempotency-Key':idempotencyKey}:{})},body});
    const data=r.data as {error?:string};
    if(generation!==store.getState().sessionGeneration)throw new SessionChangedError();
    if(!r.ok){if(r.status===401&&store.getState().user)store.dispatch(actions.signedOut());throw new Error(data.error||'Unable to complete the request');}
    return data as T;
  } catch(error) {
    if(generation!==store.getState().sessionGeneration)throw new SessionChangedError();
    throw error;
  } finally {pending.delete(controller);}
}
let loadSequence=0;
export async function reload():Promise<void>{
  const {user,sessionGeneration:generation}=store.getState();if(!user)return;
  const loadId=++loadSequence;store.dispatch(actions.loading({generation,loadId}));
  try {
    const entries=await api<Entry[]>(`/summary?timeZone=${encodeURIComponent(Intl.DateTimeFormat().resolvedOptions().timeZone)}`);
    const audit:Audit[]=[];
    store.dispatch(actions.loaded({generation,loadId,entries,audit}));
  } catch(error) {
    store.dispatch(actions.loadFailed({generation,loadId,message:(error as Error).message}));
  }
}
export async function mutateJournal(path:string,method:'POST'|'PUT'|'DELETE',body:unknown,onCommitted:()=>void,idempotencyKey?:string):Promise<void>{
  const generation=store.getState().sessionGeneration;
  const result=await api<{entry:Entry|null}>(path,method,body,idempotencyKey);
  if(generation!==store.getState().sessionGeneration)return;
  const id=result.entry?.id||path.split('?')[0].split('/').pop()!;
  store.dispatch(actions.committed({generation,id,entry:result.entry}));
  onCommitted();
  // A refresh error must never turn a committed save into a retryable POST.
  await reload();
}
