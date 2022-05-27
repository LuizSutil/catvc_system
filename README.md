# CATVC

Projeto Catvc possui os seguintes componentes

## S7 driver

s7connect driver conecta a um PLC através de tcp/ip. O arquivo de configuração tagList é o local onde consta quais variaveis serão lidas do PLC.
O driver divide as variaveis on blocos para facilitar a leitura. O driver lê os blocos, que é retornado em um array de bytes. É então gerado uma serie
de processos concorrentes para atribuir o endereço ao valor do array no processo do prometheus. 
Um outro processo abre um endpoint no http://{ip}:8080/metrics que libera os dados para serem lidos pelo prometheus.

## Prometheus

Prometheus é o principal meio de puxar dados dos drivers e dos outros processos. O arquivo prometheus.yml é onde é configurado os endpoints
de leitura e velocidade de leitura.

## Thanos

Thanos é dividio nos processos sidecar, store, querier, compactor e query frontend.

  -- Sidecar: Capta os dados do prometheus e manda para o nosso banco de dados
  
  -- Store: API Gateway para buscar os dados do sidecar/banco
  
  -- Querier: Usado para unificar todos os Sidecars 
  
  -- Query Frontend: Usado como um emulador de prometheus do thanos para o grafana, server os dados
  
  -- Compactor: Compacta os dados do banco em blocos dependendo das configurações desejadas
  
 
 ## MinIO
 
 MinIO é um banco de dados de blocos similar ao S3 que pode ser rodado localmente. Pode ser escalado horizontalmente com o uso de nginx,
 e tambem é integravel com AWS S3 caso necessario no futuro.
 
 ## Grafana
 
 Grafana é o aplicativo onde é mostrado os dados adquiridos, montando paineis e organizando usuarios.
