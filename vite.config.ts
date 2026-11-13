import {defineConfig} from 'vite';
import react from '@vitejs/plugin-react';
export default defineConfig(({mode})=>({
 plugins:[react()],
 define:{__BUILD_UTC__:JSON.stringify(new Date().toISOString().slice(0,16).replace('T',' ')+' UTC')},
 root:'web',
 build:{outDir:'../dist',emptyOutDir:true},
 server:{
  host:'127.0.0.1',port:mode==='mock'?Number(process.env.MOCK_WEB_PORT||5174):5173,strictPort:true,
  proxy:{'/api':mode==='mock'?`http://127.0.0.1:${process.env.MOCK_API_PORT||8081}`:'http://127.0.0.1:8080'}
 }
}));
