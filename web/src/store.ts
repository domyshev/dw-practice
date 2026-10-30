import { configureStore, createSlice, type PayloadAction } from '@reduxjs/toolkit';
import { useDispatch, useSelector } from 'react-redux';
export type Practice = 'prostrations' | 'diamond-mind' | 'mandala' | 'guru-yoga';
export type Entry = {id:string;userId:string;practice:Practice;large:number;small:number;repetitions:number;total:number;initial:boolean;occurredAt:string;timeZone:string;offsetMinutes:number;note:string;createdAt:string;updatedAt:string;version:number};
export type Audit = {id:string;entryId:string;action:string;at:string;before:Entry|null;after:Entry|null};
export type User = {id:string;email:string};
const slice=createSlice({name:'practice',initialState:{user:null as User|null,entries:[] as Entry[],audit:[] as Audit[],view:'overview',filter:'all'},reducers:{
 signedIn:(s,a:PayloadAction<User>)=>{s.user=a.payload},signedOut:s=>{s.user=null;s.entries=[];s.audit=[];s.view='overview';s.filter='all'},
 loaded:(s,a:PayloadAction<{entries:Entry[];audit:Audit[]}>)=>{s.entries=a.payload.entries;s.audit=a.payload.audit},
 navigate:(s,a:PayloadAction<string>)=>{s.view=a.payload;s.filter='all'},filter:(s,a:PayloadAction<string>)=>{s.filter=a.payload}
}});
export const actions=slice.actions;
export const store=configureStore({reducer:slice.reducer});
export const useAppDispatch=useDispatch.withTypes<typeof store.dispatch>();
export const useAppSelector=useSelector.withTypes<ReturnType<typeof store.getState>>();
export async function api<T>(path:string,method='GET',body?:unknown):Promise<T>{
 const r=await fetch(`/api${path}`,{method,credentials:'same-origin',headers:method!=='GET'?{'Content-Type':'application/json'}:{},body:body===undefined?undefined:JSON.stringify(body)});
 const data=await r.json();if(!r.ok){if(r.status===401&&store.getState().user)store.dispatch(actions.signedOut());throw new Error(data.error||'Unable to complete the request')};return data;
}
export async function reload(){const [entries,audit]=await Promise.all([api<Entry[]>('/entries'),api<Audit[]>('/audit')]);store.dispatch(actions.loaded({entries,audit}));}
