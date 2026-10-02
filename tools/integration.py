#!/usr/bin/env python3
"""Run isolated live protocol checks without a browser or an existing database."""
import argparse
import json
import os
from pathlib import Path
import secrets
import subprocess
import uuid

ROOT=Path(__file__).resolve().parent.parent


def main():
    parser=argparse.ArgumentParser()
    parser.add_argument('--docker-context',default='default')
    parser.add_argument('--group',choices=['core','messages','documents','vectors','configuration','all'],default='core')
    args=parser.parse_args()
    prefix=['docker','--context',args.docker_context]
    containers=[]
    cache=ROOT/'.cache';cache.mkdir(exist_ok=True)
    config_file=cache/f'live-{uuid.uuid4().hex}.json'
    password=secrets.token_urlsafe(20)

    def docker(arguments,**kwargs):
        return subprocess.check_output(prefix+arguments,text=True,**kwargs).strip()

    def start(kind,image,port,env=None,command=None):
        name='navifyne-test-'+kind+'-'+uuid.uuid4().hex[:8]
        call=['run','--detach','--rm','--name',name,'--label','io.github.ealink1.navifyne.selfcheck=true','--publish',f'127.0.0.1::{port}']
        for key,value in (env or {}).items():call+=['--env',f'{key}={value}']
        call+=[image]+(command or [])
        print(f'Starting isolated {kind}',flush=True)
        docker(call);containers.append(name)
        info=json.loads(docker(['inspect',name]))[0]
        mapped=info['NetworkSettings']['Ports'][str(port)+'/tcp'][0]['HostPort']
        return {'type':kind,'host':'127.0.0.1','port':int(mapped),'user':'','password':'','timeout':5,'queryTimeout':15}

    def run_group(group):
        configs=[]
        if group=='core':
            mysql=start('mysql','mysql:8.4',3306,{'MYSQL_ROOT_PASSWORD':password,'MYSQL_DATABASE':'navifyne_test','MYSQL_ROOT_HOST':'%'})
            mysql.update(user='root',password=password,database='navifyne_test');configs.append(mysql)
            postgres=start('postgres','postgres:17-alpine',5432,{'POSTGRES_PASSWORD':password,'POSTGRES_DB':'navifyne_test'})
            postgres.update(user='postgres',password=password,database='navifyne_test');configs.append(postgres)
            configs.append(start('redis','redis:7-alpine',6379))
        if group=='messages':
            configs.append(start('mqtt','eclipse-mosquitto:2',1883,command=['sh','-c','printf "listener 1883 0.0.0.0\\nallow_anonymous true\\n" > /tmp/navifyne.conf; exec mosquitto -c /tmp/navifyne.conf']))
            rabbit=start('rabbitmq','rabbitmq:4-management-alpine',15672,{'RABBITMQ_DEFAULT_USER':'navifyne','RABBITMQ_DEFAULT_PASS':password});rabbit.update(user='navifyne',password=password,database='/');configs.append(rabbit)
        if group=='documents':
            configs.append(start('mongodb','mongo:8',27017))
            elastic=start('elasticsearch','docker.elastic.co/elasticsearch/elasticsearch:8.19.4',9200,{'discovery.type':'single-node','xpack.security.enabled':'false','ES_JAVA_OPTS':'-Xms512m -Xmx512m'});configs.append(elastic)
        if group=='vectors':
            configs.append(start('qdrant','qdrant/qdrant:v1.14.1',6333))
            configs.append(start('chroma','chromadb/chroma:1.0.20',8000))
        if group=='configuration':
            configs.append(start('nacos','nacos/nacos-server:v2.5.1',8848,{'MODE':'standalone','NACOS_AUTH_ENABLE':'false','JVM_XMS':'256m','JVM_XMX':'512m','JVM_XMN':'128m'}))
        config_file.unlink(missing_ok=True)
        with os.fdopen(os.open(config_file,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600),'w') as output:
            json.dump(configs,output)
        env=os.environ.copy();env['NAVIFYNE_LIVE_CONFIG']=str(config_file);env['NAVIFYNE_TEST_DRIVERS']=str(ROOT/'bin/drivers')
        subprocess.run(['go','test','-tags','integration','-count=1','-v','./internal/infra/runtime','-run','TestLive'],cwd=ROOT,env=env,check=True)

    try:
        groups=['core','messages','documents','vectors','configuration'] if args.group=='all' else [args.group]
        for group in groups:
            first=len(containers)
            try:run_group(group)
            finally:
                for name in containers[first:]:
                    subprocess.run(prefix+['rm','--force',name],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
                del containers[first:]
    finally:
        config_file.unlink(missing_ok=True)
        for name in containers:
            subprocess.run(prefix+['rm','--force',name],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)


if __name__=='__main__':main()
