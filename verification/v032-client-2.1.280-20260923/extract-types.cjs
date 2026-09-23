const fs=require('fs');
const names=process.argv.slice(3);
const src=fs.readFileSync(process.argv[2],'utf8');
for(const n of names){
  let found=0;
  for(const lead of ['type '+n+' =','type '+n+'<','interface '+n+' ','interface '+n+'<']){
    let at=-1;
    while((at=src.indexOf(lead,at+1))>=0){
      found++;
      let i=src.indexOf('{',at), j=i, d=0;
      for(;j<src.length;j++){const c=src[j]; if(c==='{')d++; else if(c==='}'){d--; if(d===0)break;}}
      const body=src.slice(at,j+1).split('\n').filter(l=>!/^\s*(\*|\/\*\*|\/\/)/.test(l)&&l.trim()).join('\n');
      console.log('### '+n+'\n'+body);
    }
  }
  if(!found) console.log('### '+n+' NOT FOUND');
}
