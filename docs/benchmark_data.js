const BENCHMARK_DATA = [
  {
    "index": 1,
    "url": "https://g1.globo.com",
    "normal_kb": 2832.2,
    "pocket_kb": 24.9,
    "savings_pct": 99.1,
    "time_2g_normal_sec": 453.2,
    "time_2g_pocket_sec": 4.0
  },
  {
    "index": 2,
    "url": "https://www.uol.com.br",
    "normal_kb": 1444.5,
    "pocket_kb": 68.1,
    "savings_pct": 95.3,
    "time_2g_normal_sec": 231.1,
    "time_2g_pocket_sec": 10.9
  },
  {
    "index": 3,
    "url": "https://www.cnnbrasil.com.br",
    "normal_kb": 1292.2,
    "pocket_kb": 42.4,
    "savings_pct": 96.7,
    "time_2g_normal_sec": 206.8,
    "time_2g_pocket_sec": 6.8
  },
  {
    "index": 4,
    "url": "https://www.bbc.com/portuguese",
    "normal_kb": 937.3,
    "pocket_kb": 33.3,
    "savings_pct": 96.4,
    "time_2g_normal_sec": 150.0,
    "time_2g_pocket_sec": 5.3
  },
  {
    "index": 5,
    "url": "https://www.folha.uol.com.br",
    "normal_kb": 1342.4,
    "pocket_kb": 48.1,
    "savings_pct": 96.4,
    "time_2g_normal_sec": 214.8,
    "time_2g_pocket_sec": 7.7
  },
  {
    "index": 6,
    "url": "https://www.estadao.com.br",
    "normal_kb": 3123.9,
    "pocket_kb": 44.0,
    "savings_pct": 98.6,
    "time_2g_normal_sec": 499.8,
    "time_2g_pocket_sec": 7.0
  },
  {
    "index": 7,
    "url": "https://www.r7.com",
    "normal_kb": 6234.1,
    "pocket_kb": 80.0,
    "savings_pct": 98.7,
    "time_2g_normal_sec": 997.5,
    "time_2g_pocket_sec": 12.8
  },
  {
    "index": 8,
    "url": "https://www.terra.com.br",
    "normal_kb": 2607.2,
    "pocket_kb": 17.0,
    "savings_pct": 99.3,
    "time_2g_normal_sec": 417.2,
    "time_2g_pocket_sec": 2.7
  },
  {
    "index": 9,
    "url": "https://noticias.uol.com.br",
    "normal_kb": 2160.0,
    "pocket_kb": 71.3,
    "savings_pct": 96.7,
    "time_2g_normal_sec": 345.6,
    "time_2g_pocket_sec": 11.4
  },
  {
    "index": 10,
    "url": "https://ge.globo.com",
    "normal_kb": 3036.5,
    "pocket_kb": 29.3,
    "savings_pct": 99.0,
    "time_2g_normal_sec": 485.8,
    "time_2g_pocket_sec": 4.7
  },
  {
    "index": 11,
    "url": "https://oglobo.globo.com",
    "normal_kb": 2768.9,
    "pocket_kb": 77.8,
    "savings_pct": 97.2,
    "time_2g_normal_sec": 443.0,
    "time_2g_pocket_sec": 12.4
  },
  {
    "index": 12,
    "url": "https://www.metropoles.com",
    "normal_kb": 3173.2,
    "pocket_kb": 56.6,
    "savings_pct": 98.2,
    "time_2g_normal_sec": 507.7,
    "time_2g_pocket_sec": 9.1
  },
  {
    "index": 13,
    "url": "https://www.poder360.com.br",
    "normal_kb": 823.6,
    "pocket_kb": 33.8,
    "savings_pct": 95.9,
    "time_2g_normal_sec": 131.8,
    "time_2g_pocket_sec": 5.4
  },
  {
    "index": 14,
    "url": "https://www.gazetadopovo.com.br",
    "normal_kb": 1915.8,
    "pocket_kb": 24.0,
    "savings_pct": 98.7,
    "time_2g_normal_sec": 306.5,
    "time_2g_pocket_sec": 3.8
  },
  {
    "index": 15,
    "url": "https://www.bbc.com/news",
    "normal_kb": 1315.5,
    "pocket_kb": 18.7,
    "savings_pct": 98.6,
    "time_2g_normal_sec": 210.5,
    "time_2g_pocket_sec": 3.0
  },
  {
    "index": 16,
    "url": "https://edition.cnn.com",
    "normal_kb": 15970.7,
    "pocket_kb": 235.1,
    "savings_pct": 98.5,
    "time_2g_normal_sec": 2555.3,
    "time_2g_pocket_sec": 37.6
  },
  {
    "index": 17,
    "url": "https://www.theguardian.com/internatio",
    "normal_kb": 3867.0,
    "pocket_kb": 122.2,
    "savings_pct": 96.8,
    "time_2g_normal_sec": 618.7,
    "time_2g_pocket_sec": 19.6
  },
  {
    "index": 18,
    "url": "https://www.nytimes.com",
    "normal_kb": 3.2,
    "pocket_kb": 0.2,
    "savings_pct": 93.8,
    "time_2g_normal_sec": 0.5,
    "time_2g_pocket_sec": 0.0
  },
  {
    "index": 19,
    "url": "https://www.reuters.com",
    "normal_kb": 1858.5,
    "pocket_kb": 39.7,
    "savings_pct": 97.9,
    "time_2g_normal_sec": 297.4,
    "time_2g_pocket_sec": 6.4
  },
  {
    "index": 20,
    "url": "https://elpais.com/america",
    "normal_kb": 614.3,
    "pocket_kb": 56.8,
    "savings_pct": 90.8,
    "time_2g_normal_sec": 98.3,
    "time_2g_pocket_sec": 9.1
  },
  {
    "index": 21,
    "url": "https://www.lemonde.fr",
    "normal_kb": 2445.8,
    "pocket_kb": 78.6,
    "savings_pct": 96.8,
    "time_2g_normal_sec": 391.3,
    "time_2g_pocket_sec": 12.6
  },
  {
    "index": 22,
    "url": "https://www.aljazeera.com",
    "normal_kb": 1821.9,
    "pocket_kb": 14.3,
    "savings_pct": 99.2,
    "time_2g_normal_sec": 291.5,
    "time_2g_pocket_sec": 2.3
  },
  {
    "index": 23,
    "url": "https://dw.com/pt-br",
    "normal_kb": 579.0,
    "pocket_kb": 13.1,
    "savings_pct": 97.7,
    "time_2g_normal_sec": 92.6,
    "time_2g_pocket_sec": 2.1
  },
  {
    "index": 24,
    "url": "https://www.bloomberg.com",
    "normal_kb": 30.2,
    "pocket_kb": 2.0,
    "savings_pct": 93.4,
    "time_2g_normal_sec": 4.8,
    "time_2g_pocket_sec": 0.3
  },
  {
    "index": 25,
    "url": "https://www.forbes.com",
    "normal_kb": 2022.5,
    "pocket_kb": 80.4,
    "savings_pct": 96.0,
    "time_2g_normal_sec": 323.6,
    "time_2g_pocket_sec": 12.9
  },
  {
    "index": 26,
    "url": "https://pt.wikipedia.org/wiki/Primeiro",
    "normal_kb": 725.8,
    "pocket_kb": 40.9,
    "savings_pct": 94.4,
    "time_2g_normal_sec": 116.1,
    "time_2g_pocket_sec": 6.5
  },
  {
    "index": 27,
    "url": "https://pt.wikipedia.org/wiki/Brasil",
    "normal_kb": 6487.2,
    "pocket_kb": 319.8,
    "savings_pct": 95.1,
    "time_2g_normal_sec": 1038.0,
    "time_2g_pocket_sec": 51.2
  },
  {
    "index": 28,
    "url": "https://pt.wikipedia.org/wiki/Sa%C3%BA",
    "normal_kb": 472.4,
    "pocket_kb": 31.7,
    "savings_pct": 93.3,
    "time_2g_normal_sec": 75.6,
    "time_2g_pocket_sec": 5.1
  },
  {
    "index": 29,
    "url": "https://pt.wikipedia.org/wiki/Crise_cl",
    "normal_kb": 1200.5,
    "pocket_kb": 70.4,
    "savings_pct": 94.1,
    "time_2g_normal_sec": 192.1,
    "time_2g_pocket_sec": 11.3
  },
  {
    "index": 30,
    "url": "https://pt.wikipedia.org/wiki/Dengue",
    "normal_kb": 1635.1,
    "pocket_kb": 96.1,
    "savings_pct": 94.1,
    "time_2g_normal_sec": 261.6,
    "time_2g_pocket_sec": 15.4
  },
  {
    "index": 31,
    "url": "https://en.wikipedia.org/wiki/Main_Pag",
    "normal_kb": 506.5,
    "pocket_kb": 46.9,
    "savings_pct": 90.7,
    "time_2g_normal_sec": 81.0,
    "time_2g_pocket_sec": 7.5
  },
  {
    "index": 32,
    "url": "https://en.wikipedia.org/wiki/Emergenc",
    "normal_kb": 1860.4,
    "pocket_kb": 97.4,
    "savings_pct": 94.8,
    "time_2g_normal_sec": 297.7,
    "time_2g_pocket_sec": 15.6
  },
  {
    "index": 33,
    "url": "https://en.wikipedia.org/wiki/Water_pu",
    "normal_kb": 1266.1,
    "pocket_kb": 77.4,
    "savings_pct": 93.9,
    "time_2g_normal_sec": 202.6,
    "time_2g_pocket_sec": 12.4
  },
  {
    "index": 34,
    "url": "https://www.britannica.com",
    "normal_kb": 2312.8,
    "pocket_kb": 20.6,
    "savings_pct": 99.1,
    "time_2g_normal_sec": 370.0,
    "time_2g_pocket_sec": 3.3
  },
  {
    "index": 35,
    "url": "https://pt.wikihow.com/Tratar-uma-Quei",
    "normal_kb": 598.8,
    "pocket_kb": 37.6,
    "savings_pct": 93.7,
    "time_2g_normal_sec": 95.8,
    "time_2g_pocket_sec": 6.0
  },
  {
    "index": 36,
    "url": "https://pt.wikihow.com/Purificar-%C3%8",
    "normal_kb": 1363.1,
    "pocket_kb": 53.6,
    "savings_pct": 96.1,
    "time_2g_normal_sec": 218.1,
    "time_2g_pocket_sec": 8.6
  },
  {
    "index": 37,
    "url": "https://www.scielo.br",
    "normal_kb": 1682.2,
    "pocket_kb": 8.9,
    "savings_pct": 99.5,
    "time_2g_normal_sec": 269.2,
    "time_2g_pocket_sec": 1.4
  },
  {
    "index": 38,
    "url": "https://brasilescola.uol.com.br",
    "normal_kb": 1150.1,
    "pocket_kb": 15.2,
    "savings_pct": 98.7,
    "time_2g_normal_sec": 184.0,
    "time_2g_pocket_sec": 2.4
  },
  {
    "index": 39,
    "url": "https://mundoeducacao.uol.com.br",
    "normal_kb": 762.3,
    "pocket_kb": 8.0,
    "savings_pct": 99.0,
    "time_2g_normal_sec": 122.0,
    "time_2g_pocket_sec": 1.3
  },
  {
    "index": 40,
    "url": "https://www.todamateria.com.br",
    "normal_kb": 1482.2,
    "pocket_kb": 73.5,
    "savings_pct": 95.0,
    "time_2g_normal_sec": 237.2,
    "time_2g_pocket_sec": 11.8
  },
  {
    "index": 41,
    "url": "https://www.stoodi.com.br",
    "normal_kb": 1893.5,
    "pocket_kb": 15.5,
    "savings_pct": 99.2,
    "time_2g_normal_sec": 303.0,
    "time_2g_pocket_sec": 2.5
  },
  {
    "index": 42,
    "url": "https://www.w3schools.com",
    "normal_kb": 1010.2,
    "pocket_kb": 43.3,
    "savings_pct": 95.7,
    "time_2g_normal_sec": 161.6,
    "time_2g_pocket_sec": 6.9
  },
  {
    "index": 43,
    "url": "https://developer.mozilla.org/pt-BR/",
    "normal_kb": 347.3,
    "pocket_kb": 8.3,
    "savings_pct": 97.6,
    "time_2g_normal_sec": 55.6,
    "time_2g_pocket_sec": 1.3
  },
  {
    "index": 44,
    "url": "https://stackoverflow.com",
    "normal_kb": 11.3,
    "pocket_kb": 0.8,
    "savings_pct": 92.9,
    "time_2g_normal_sec": 1.8,
    "time_2g_pocket_sec": 0.1
  },
  {
    "index": 45,
    "url": "https://arxiv.org",
    "normal_kb": 191.8,
    "pocket_kb": 6.6,
    "savings_pct": 96.6,
    "time_2g_normal_sec": 30.7,
    "time_2g_pocket_sec": 1.1
  },
  {
    "index": 46,
    "url": "https://www.gov.br",
    "normal_kb": 4916.3,
    "pocket_kb": 17.4,
    "savings_pct": 99.6,
    "time_2g_normal_sec": 786.6,
    "time_2g_pocket_sec": 2.8
  },
  {
    "index": 47,
    "url": "https://www.gov.br/saude/pt-br",
    "normal_kb": 3336.5,
    "pocket_kb": 20.5,
    "savings_pct": 99.4,
    "time_2g_normal_sec": 533.8,
    "time_2g_pocket_sec": 3.3
  },
  {
    "index": 48,
    "url": "https://www.gov.br/receitafederal/pt-b",
    "normal_kb": 8343.5,
    "pocket_kb": 32.5,
    "savings_pct": 99.6,
    "time_2g_normal_sec": 1335.0,
    "time_2g_pocket_sec": 5.2
  },
  {
    "index": 49,
    "url": "https://www.ibge.gov.br",
    "normal_kb": 874.7,
    "pocket_kb": 24.6,
    "savings_pct": 97.2,
    "time_2g_normal_sec": 140.0,
    "time_2g_pocket_sec": 3.9
  },
  {
    "index": 50,
    "url": "https://portal.fiocruz.br",
    "normal_kb": 3728.4,
    "pocket_kb": 43.7,
    "savings_pct": 98.8,
    "time_2g_normal_sec": 596.5,
    "time_2g_pocket_sec": 7.0
  },
  {
    "index": 51,
    "url": "https://www.who.int",
    "normal_kb": 2422.9,
    "pocket_kb": 19.4,
    "savings_pct": 99.2,
    "time_2g_normal_sec": 387.7,
    "time_2g_pocket_sec": 3.1
  },
  {
    "index": 52,
    "url": "https://www.un.org",
    "normal_kb": 1178.8,
    "pocket_kb": 2.0,
    "savings_pct": 99.8,
    "time_2g_normal_sec": 188.6,
    "time_2g_pocket_sec": 0.3
  },
  {
    "index": 53,
    "url": "https://www.climatempo.com.br",
    "normal_kb": 2865.2,
    "pocket_kb": 18.9,
    "savings_pct": 99.3,
    "time_2g_normal_sec": 458.4,
    "time_2g_pocket_sec": 3.0
  },
  {
    "index": 54,
    "url": "https://portal.inmet.gov.br",
    "normal_kb": 1983.9,
    "pocket_kb": 6.7,
    "savings_pct": 99.7,
    "time_2g_normal_sec": 317.4,
    "time_2g_pocket_sec": 1.1
  },
  {
    "index": 55,
    "url": "https://www.inpe.br",
    "normal_kb": 1120.4,
    "pocket_kb": 18.2,
    "savings_pct": 98.4,
    "time_2g_normal_sec": 179.3,
    "time_2g_pocket_sec": 2.9
  },
  {
    "index": 56,
    "url": "https://www.defesacivil.sp.gov.br",
    "normal_kb": 8039.0,
    "pocket_kb": 12.2,
    "savings_pct": 99.8,
    "time_2g_normal_sec": 1286.2,
    "time_2g_pocket_sec": 2.0
  },
  {
    "index": 57,
    "url": "https://www.icrc.org/pt",
    "normal_kb": 1650.0,
    "pocket_kb": 21.4,
    "savings_pct": 98.7,
    "time_2g_normal_sec": 264.0,
    "time_2g_pocket_sec": 3.4
  },
  {
    "index": 58,
    "url": "https://www.msf.org.br",
    "normal_kb": 7547.1,
    "pocket_kb": 40.9,
    "savings_pct": 99.5,
    "time_2g_normal_sec": 1207.5,
    "time_2g_pocket_sec": 6.5
  },
  {
    "index": 59,
    "url": "https://www.icrc.org/pt",
    "normal_kb": 21.0,
    "pocket_kb": 1.5,
    "savings_pct": 92.9,
    "time_2g_normal_sec": 3.4,
    "time_2g_pocket_sec": 0.2
  },
  {
    "index": 60,
    "url": "https://www.unicef.org/brazil/",
    "normal_kb": 18798.7,
    "pocket_kb": 18.4,
    "savings_pct": 99.9,
    "time_2g_normal_sec": 3007.8,
    "time_2g_pocket_sec": 2.9
  },
  {
    "index": 61,
    "url": "https://github.com",
    "normal_kb": 2263.4,
    "pocket_kb": 25.1,
    "savings_pct": 98.9,
    "time_2g_normal_sec": 362.1,
    "time_2g_pocket_sec": 4.0
  },
  {
    "index": 62,
    "url": "https://news.ycombinator.com",
    "normal_kb": 72.9,
    "pocket_kb": 5.4,
    "savings_pct": 92.6,
    "time_2g_normal_sec": 11.7,
    "time_2g_pocket_sec": 0.9
  },
  {
    "index": 63,
    "url": "https://dev.to",
    "normal_kb": 460.2,
    "pocket_kb": 22.2,
    "savings_pct": 95.2,
    "time_2g_normal_sec": 73.6,
    "time_2g_pocket_sec": 3.6
  },
  {
    "index": 64,
    "url": "https://techcrunch.com",
    "normal_kb": 2152.0,
    "pocket_kb": 47.4,
    "savings_pct": 97.8,
    "time_2g_normal_sec": 344.3,
    "time_2g_pocket_sec": 7.6
  },
  {
    "index": 65,
    "url": "https://www.theverge.com",
    "normal_kb": 2265.5,
    "pocket_kb": 43.1,
    "savings_pct": 98.1,
    "time_2g_normal_sec": 362.5,
    "time_2g_pocket_sec": 6.9
  },
  {
    "index": 66,
    "url": "https://arstechnica.com",
    "normal_kb": 20.6,
    "pocket_kb": 2.1,
    "savings_pct": 89.8,
    "time_2g_normal_sec": 3.3,
    "time_2g_pocket_sec": 0.3
  },
  {
    "index": 67,
    "url": "https://www.wired.com",
    "normal_kb": 17819.8,
    "pocket_kb": 65.7,
    "savings_pct": 99.6,
    "time_2g_normal_sec": 2851.2,
    "time_2g_pocket_sec": 10.5
  },
  {
    "index": 68,
    "url": "https://www.engadget.com",
    "normal_kb": 2.0,
    "pocket_kb": 0.6,
    "savings_pct": 70.0,
    "time_2g_normal_sec": 0.3,
    "time_2g_pocket_sec": 0.1
  },
  {
    "index": 69,
    "url": "https://tecnoblog.net",
    "normal_kb": 2115.6,
    "pocket_kb": 30.6,
    "savings_pct": 98.6,
    "time_2g_normal_sec": 338.5,
    "time_2g_pocket_sec": 4.9
  },
  {
    "index": 70,
    "url": "https://canaltech.com.br",
    "normal_kb": 669.6,
    "pocket_kb": 17.2,
    "savings_pct": 97.4,
    "time_2g_normal_sec": 107.1,
    "time_2g_pocket_sec": 2.8
  },
  {
    "index": 71,
    "url": "https://olhardigital.com.br",
    "normal_kb": 314.9,
    "pocket_kb": 23.3,
    "savings_pct": 92.6,
    "time_2g_normal_sec": 50.4,
    "time_2g_pocket_sec": 3.7
  },
  {
    "index": 72,
    "url": "https://meiobit.com",
    "normal_kb": 3127.8,
    "pocket_kb": 12.5,
    "savings_pct": 99.6,
    "time_2g_normal_sec": 500.4,
    "time_2g_pocket_sec": 2.0
  },
  {
    "index": 73,
    "url": "https://manualdousuario.net",
    "normal_kb": 190.0,
    "pocket_kb": 11.4,
    "savings_pct": 94.0,
    "time_2g_normal_sec": 30.4,
    "time_2g_pocket_sec": 1.8
  },
  {
    "index": 74,
    "url": "https://distrowatch.com",
    "normal_kb": 0.5,
    "pocket_kb": 0.2,
    "savings_pct": 60.0,
    "time_2g_normal_sec": 0.1,
    "time_2g_pocket_sec": 0.0
  },
  {
    "index": 75,
    "url": "https://kernel.org",
    "normal_kb": 43.5,
    "pocket_kb": 2.6,
    "savings_pct": 94.0,
    "time_2g_normal_sec": 7.0,
    "time_2g_pocket_sec": 0.4
  },
  {
    "index": 76,
    "url": "https://python.org",
    "normal_kb": 593.7,
    "pocket_kb": 10.0,
    "savings_pct": 98.3,
    "time_2g_normal_sec": 95.0,
    "time_2g_pocket_sec": 1.6
  },
  {
    "index": 77,
    "url": "https://go.dev",
    "normal_kb": 623.3,
    "pocket_kb": 8.5,
    "savings_pct": 98.6,
    "time_2g_normal_sec": 99.7,
    "time_2g_pocket_sec": 1.4
  },
  {
    "index": 78,
    "url": "https://rust-lang.org",
    "normal_kb": 39.6,
    "pocket_kb": 4.3,
    "savings_pct": 89.1,
    "time_2g_normal_sec": 6.3,
    "time_2g_pocket_sec": 0.7
  },
  {
    "index": 79,
    "url": "https://nodejs.org",
    "normal_kb": 1457.2,
    "pocket_kb": 5.5,
    "savings_pct": 99.6,
    "time_2g_normal_sec": 233.2,
    "time_2g_pocket_sec": 0.9
  },
  {
    "index": 80,
    "url": "https://www.apache.org",
    "normal_kb": 1159.1,
    "pocket_kb": 7.4,
    "savings_pct": 99.4,
    "time_2g_normal_sec": 185.5,
    "time_2g_pocket_sec": 1.2
  },
  {
    "index": 81,
    "url": "https://www.infomoney.com.br",
    "normal_kb": 2018.8,
    "pocket_kb": 82.8,
    "savings_pct": 95.9,
    "time_2g_normal_sec": 323.0,
    "time_2g_pocket_sec": 13.2
  },
  {
    "index": 82,
    "url": "https://valor.globo.com",
    "normal_kb": 4993.2,
    "pocket_kb": 67.2,
    "savings_pct": 98.7,
    "time_2g_normal_sec": 798.9,
    "time_2g_pocket_sec": 10.8
  },
  {
    "index": 83,
    "url": "https://investnews.com.br",
    "normal_kb": 1948.7,
    "pocket_kb": 19.5,
    "savings_pct": 99.0,
    "time_2g_normal_sec": 311.8,
    "time_2g_pocket_sec": 3.1
  },
  {
    "index": 84,
    "url": "https://br.investing.com",
    "normal_kb": 0.3,
    "pocket_kb": 0.1,
    "savings_pct": 66.7,
    "time_2g_normal_sec": 0.0,
    "time_2g_pocket_sec": 0.0
  },
  {
    "index": 85,
    "url": "https://finance.yahoo.com",
    "normal_kb": 2837.7,
    "pocket_kb": 40.6,
    "savings_pct": 98.6,
    "time_2g_normal_sec": 454.0,
    "time_2g_pocket_sec": 6.5
  },
  {
    "index": 86,
    "url": "https://exame.com",
    "normal_kb": 1483.7,
    "pocket_kb": 20.1,
    "savings_pct": 98.6,
    "time_2g_normal_sec": 237.4,
    "time_2g_pocket_sec": 3.2
  },
  {
    "index": 87,
    "url": "https://epocanegocios.globo.com",
    "normal_kb": 1873.8,
    "pocket_kb": 50.7,
    "savings_pct": 97.3,
    "time_2g_normal_sec": 299.8,
    "time_2g_pocket_sec": 8.1
  },
  {
    "index": 88,
    "url": "https://www.moneytimes.com.br",
    "normal_kb": 2046.5,
    "pocket_kb": 17.3,
    "savings_pct": 99.2,
    "time_2g_normal_sec": 327.4,
    "time_2g_pocket_sec": 2.8
  },
  {
    "index": 89,
    "url": "https://suno.com.br",
    "normal_kb": 947.6,
    "pocket_kb": 18.0,
    "savings_pct": 98.1,
    "time_2g_normal_sec": 151.6,
    "time_2g_pocket_sec": 2.9
  },
  {
    "index": 90,
    "url": "https://statusinvest.com.br",
    "normal_kb": 1870.6,
    "pocket_kb": 34.8,
    "savings_pct": 98.1,
    "time_2g_normal_sec": 299.3,
    "time_2g_pocket_sec": 5.6
  },
  {
    "index": 91,
    "url": "https://medium.com",
    "normal_kb": 396.3,
    "pocket_kb": 6.8,
    "savings_pct": 98.3,
    "time_2g_normal_sec": 63.4,
    "time_2g_pocket_sec": 1.1
  },
  {
    "index": 92,
    "url": "https://www.reddit.com/r/brasil",
    "normal_kb": 251.9,
    "pocket_kb": 9.4,
    "savings_pct": 96.3,
    "time_2g_normal_sec": 40.3,
    "time_2g_pocket_sec": 1.5
  },
  {
    "index": 93,
    "url": "https://www.reddit.com/r/technology",
    "normal_kb": 1495.8,
    "pocket_kb": 81.2,
    "savings_pct": 94.6,
    "time_2g_normal_sec": 239.3,
    "time_2g_pocket_sec": 13.0
  },
  {
    "index": 94,
    "url": "https://slashdot.org",
    "normal_kb": 182.8,
    "pocket_kb": 35.1,
    "savings_pct": 80.8,
    "time_2g_normal_sec": 29.2,
    "time_2g_pocket_sec": 5.6
  },
  {
    "index": 95,
    "url": "https://krebsonsecurity.com",
    "normal_kb": 179.2,
    "pocket_kb": 27.7,
    "savings_pct": 84.5,
    "time_2g_normal_sec": 28.7,
    "time_2g_pocket_sec": 4.4
  },
  {
    "index": 96,
    "url": "https://arstechnica.com/science/",
    "normal_kb": 20.6,
    "pocket_kb": 2.1,
    "savings_pct": 89.8,
    "time_2g_normal_sec": 3.3,
    "time_2g_pocket_sec": 0.3
  },
  {
    "index": 97,
    "url": "https://nature.com",
    "normal_kb": 648.7,
    "pocket_kb": 19.4,
    "savings_pct": 97.0,
    "time_2g_normal_sec": 103.8,
    "time_2g_pocket_sec": 3.1
  },
  {
    "index": 98,
    "url": "https://scientificamerican.com",
    "normal_kb": 1517.9,
    "pocket_kb": 19.4,
    "savings_pct": 98.7,
    "time_2g_normal_sec": 242.9,
    "time_2g_pocket_sec": 3.1
  },
  {
    "index": 99,
    "url": "https://nationalgeographic.com",
    "normal_kb": 1644.9,
    "pocket_kb": 19.5,
    "savings_pct": 98.8,
    "time_2g_normal_sec": 263.2,
    "time_2g_pocket_sec": 3.1
  },
  {
    "index": 100,
    "url": "https://www.worldwildlife.org",
    "normal_kb": 817.2,
    "pocket_kb": 17.3,
    "savings_pct": 97.9,
    "time_2g_normal_sec": 130.8,
    "time_2g_pocket_sec": 2.8
  }
];
